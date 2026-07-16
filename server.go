package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash/fnv"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/a-h/templ"
	"github.com/gorilla/websocket"
	"github.com/hacel/htmxchat/templates"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

const (
	recentMessageLimit = 20
	maxMessageRunes    = 1000
	maxPayloadBytes    = 16 * 1024
	writeTimeout       = 10 * time.Second
	pongTimeout        = 60 * time.Second
	pingInterval       = 45 * time.Second
	clientQueueSize    = 32
)

var colors = []string{
	"amber", "blue", "cyan", "emerald", "fuchsia", "gray", "green", "indigo",
	"lime", "neutral", "orange", "pink", "purple", "red", "rose", "sky",
	"slate", "stone", "teal", "violet", "yellow", "zinc",
}

//go:embed static
var staticAssets embed.FS

type incomingMessage struct {
	Content string `json:"chat_message"`
}

type wsClient struct {
	connection *websocket.Conn
	send       chan []byte
	done       chan struct{}
	closeOnce  sync.Once
}

func newWSClient(connection *websocket.Conn) *wsClient {
	return &wsClient{
		connection: connection,
		send:       make(chan []byte, clientQueueSize),
		done:       make(chan struct{}),
	}
}

func (client *wsClient) enqueue(payload []byte) bool {
	select {
	case <-client.done:
		return false
	default:
	}
	select {
	case client.send <- payload:
		return true
	case <-client.done:
		return false
	default:
		return false
	}
}

func (client *wsClient) close(code int, reason string) {
	client.closeOnce.Do(func() {
		close(client.done)
		_ = client.connection.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(code, reason),
			time.Now().Add(writeTimeout),
		)
		_ = client.connection.Close()
	})
}

type chatServer struct {
	database  *sql.DB
	upgrader  websocket.Upgrader
	clientsMu sync.RWMutex
	clients   map[*wsClient]struct{}
}

func newChatServer(database *sql.DB) *chatServer {
	return &chatServer{
		database: database,
		clients:  make(map[*wsClient]struct{}),
	}
}

func newHTTPServer(chat *chatServer) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.IPExtractor = echo.ExtractIPFromRealIPHeader(
		echo.TrustLinkLocal(false),
		echo.TrustPrivateNet(false),
	)
	e.Server.ReadHeaderTimeout = 5 * time.Second
	e.Server.IdleTimeout = 60 * time.Second
	e.Server.MaxHeaderBytes = 16 * 1024
	e.Use(middleware.RequestID())
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.BodyLimit("16K"))
	e.Use(middleware.SecureWithConfig(middleware.SecureConfig{
		XSSProtection:         "0",
		ContentTypeNosniff:    "nosniff",
		XFrameOptions:         "DENY",
		ReferrerPolicy:        "same-origin",
		ContentSecurityPolicy: "default-src 'self'; connect-src 'self' ws: wss:; script-src 'self'; style-src 'self'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'",
	}))

	e.StaticFS("/static/", echo.MustSubFS(staticAssets, "static"))
	e.GET("/healthz", func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})
	e.GET("/ws", chat.handleWebSocket)
	e.GET("/", func(c echo.Context) error {
		return render(c, http.StatusOK, templates.Index())
	})
	return e
}

func (chat *chatServer) handleWebSocket(c echo.Context) error {
	messages, err := recentMessages(c.Request().Context(), chat.database, recentMessageLimit)
	if err != nil {
		return err
	}
	history, err := renderMessages(c.Request().Context(), messages)
	if err != nil {
		return err
	}

	connection, err := chat.upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	client := newWSClient(connection)
	connection.SetReadLimit(maxPayloadBytes)
	if err := connection.SetReadDeadline(time.Now().Add(pongTimeout)); err != nil {
		client.close(websocket.CloseInternalServerErr, "failed to configure connection")
		return nil
	}
	connection.SetPongHandler(func(string) error {
		return connection.SetReadDeadline(time.Now().Add(pongTimeout))
	})

	if len(history) > 0 && !client.enqueue(history) {
		client.close(websocket.CloseInternalServerErr, "failed to queue history")
		return nil
	}
	chat.addClient(client)
	go chat.writePump(client)
	defer func() {
		chat.removeClient(client)
		client.close(websocket.CloseNormalClosure, "")
	}()

	author := authorID(c.RealIP() + "\x00" + c.Request().UserAgent())
	for {
		_, payload, err := connection.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				c.Logger().Warnf("websocket read failed: %v", err)
			}
			return nil
		}

		content, err := parseIncomingMessage(payload)
		if err != nil {
			if errors.Is(err, errMessageTooLong) {
				client.close(websocket.CloseMessageTooBig, err.Error())
				return nil
			}
			continue
		}
		message := templates.Message{
			Time:    time.Now().UTC(),
			Author:  author,
			Color:   colorFor(author),
			Content: content,
		}
		if err := saveMessage(c.Request().Context(), chat.database, message); err != nil {
			c.Logger().Error(err)
			continue
		}
		rendered, err := renderMessages(c.Request().Context(), []templates.Message{message})
		if err != nil {
			c.Logger().Error(err)
			continue
		}
		chat.broadcast(rendered)
	}
}

func (chat *chatServer) writePump(client *wsClient) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	defer func() {
		chat.removeClient(client)
		client.close(websocket.CloseGoingAway, "connection lost")
	}()
	for {
		select {
		case payload := <-client.send:
			if err := writeWebSocketMessage(client.connection, websocket.TextMessage, payload); err != nil {
				return
			}
		case <-ticker.C:
			if err := writeWebSocketMessage(client.connection, websocket.PingMessage, nil); err != nil {
				return
			}
		case <-client.done:
			return
		}
	}
}

func writeWebSocketMessage(connection *websocket.Conn, messageType int, payload []byte) error {
	if err := connection.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return err
	}
	return connection.WriteMessage(messageType, payload)
}

func (chat *chatServer) addClient(client *wsClient) {
	chat.clientsMu.Lock()
	chat.clients[client] = struct{}{}
	chat.clientsMu.Unlock()
}

func (chat *chatServer) removeClient(client *wsClient) {
	chat.clientsMu.Lock()
	delete(chat.clients, client)
	chat.clientsMu.Unlock()
}

func (chat *chatServer) clientSnapshot() []*wsClient {
	chat.clientsMu.RLock()
	defer chat.clientsMu.RUnlock()
	clients := make([]*wsClient, 0, len(chat.clients))
	for client := range chat.clients {
		clients = append(clients, client)
	}
	return clients
}

func (chat *chatServer) broadcast(payload []byte) {
	for _, client := range chat.clientSnapshot() {
		if !client.enqueue(payload) {
			chat.removeClient(client)
			client.close(websocket.ClosePolicyViolation, "client is too slow")
		}
	}
}

func (chat *chatServer) closeAll() {
	for _, client := range chat.clientSnapshot() {
		chat.removeClient(client)
		client.close(websocket.CloseServiceRestart, "server restarting")
	}
}

func parseIncomingMessage(payload []byte) (string, error) {
	var incoming incomingMessage
	if err := json.Unmarshal(payload, &incoming); err != nil {
		return "", err
	}
	if strings.TrimSpace(incoming.Content) == "" {
		return "", errors.New("message must not be empty")
	}
	if utf8.RuneCountInString(incoming.Content) > maxMessageRunes {
		return "", errMessageTooLong
	}
	return incoming.Content, nil
}

var errMessageTooLong = errors.New("message exceeds 1000 characters")

func authorID(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:4])
}

func colorFor(value string) string {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(value))
	return colors[int(hash.Sum32()%uint32(len(colors)))]
}

func renderMessages(ctx context.Context, messages []templates.Message) ([]byte, error) {
	var buffer bytes.Buffer
	for i := range messages {
		if err := templates.RenderMessage(&messages[i]).Render(ctx, &buffer); err != nil {
			return nil, err
		}
	}
	return buffer.Bytes(), nil
}

func render(ctx echo.Context, statusCode int, component templ.Component) error {
	buffer := templ.GetBuffer()
	defer templ.ReleaseBuffer(buffer)
	if err := component.Render(ctx.Request().Context(), buffer); err != nil {
		return err
	}
	return ctx.HTML(statusCode, buffer.String())
}
