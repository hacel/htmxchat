package main

import (
	"bytes"
	"crypto/md5"
	"database/sql"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"sync"
	"time"

	"github.com/a-h/templ"
	"github.com/gorilla/websocket"
	"github.com/hacel/htmxchat/templates"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	_ "github.com/mattn/go-sqlite3"
)

func shortHash(str string) string {
	hash := md5.Sum([]byte(str))
	return fmt.Sprintf("%x", hash[:4])
}

func colorizationer(str string) string {
	colors := map[int]string{
		1:  "amber",
		2:  "blue",
		3:  "cyan",
		4:  "emerald",
		5:  "fuchsia",
		6:  "gray",
		7:  "green",
		8:  "indigo",
		9:  "lime",
		10: "neutral",
		11: "orange",
		12: "pink",
		13: "purple",
		14: "red",
		15: "rose",
		16: "sky",
		17: "slate",
		18: "stone",
		19: "teal",
		20: "violet",
		21: "yellow",
		22: "zinc",
	}
	hash := fnv.New32()
	hash.Write([]byte(str))
	key := hash.Sum32()
	return colors[int(key)%len(colors)]
}

var (
	upgrader = websocket.Upgrader{}
	sockets  = map[*websocket.Conn]bool{}
	mu       = &sync.RWMutex{}
	db       *sql.DB
)

func ws(c echo.Context) error {
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer ws.Close()

	mu.Lock()
	sockets[ws] = true
	fmt.Println("Current chatters:", len(sockets))
	mu.Unlock()
	defer func() {
		mu.Lock()
		delete(sockets, ws)
		fmt.Println("Current chatters:", len(sockets))
		mu.Unlock()
	}()

	msg := &templates.Message{}
	buf := &bytes.Buffer{}

	// Dump last few messages into the chat box
	rows, err := db.Query(`SELECT * FROM (SELECT * FROM chats ORDER BY TIME DESC LIMIT 20) ORDER BY time ASC`)
	if err != nil {
		c.Logger().Error(err)
		return err
	}
	for rows.Next() {
		if err := rows.Scan(&msg.Author, &msg.Content, &msg.Time); err != nil {
			c.Logger().Error(err)
			return err
		}
		msg.Color = colorizationer(msg.Author)
		if err := templates.RenderMessage(msg).Render(c.Request().Context(), buf); err != nil {
			c.Logger().Error(err)
			return err
		}
		if err := ws.WriteMessage(websocket.TextMessage, buf.Bytes()); err != nil {
			c.Logger().Error(err)
			return err
		}
		buf.Reset()
	}

	// Get author and colorize
	author := shortHash(c.RealIP() + c.Request().UserAgent())
	color := colorizationer(author)
	msg.Author = author
	msg.Color = color

	for {
		// Read
		_, data, err := ws.ReadMessage()
		if err != nil {
			return nil
		}

		// Parse
		if err := json.Unmarshal(data, msg); err != nil {
			c.Logger().Error(err)
			return err
		}
		msg.Time = time.Now()
		fmt.Printf("Recieved: %+v\n", msg)

		// Generate message from template and write to all sockets
		if err := templates.RenderMessage(msg).Render(c.Request().Context(), buf); err != nil {
			c.Logger().Error(err)
			return err
		}
		mu.RLock()
		for s := range sockets {
			if err := s.WriteMessage(websocket.TextMessage, buf.Bytes()); err != nil {
				c.Logger().Error(err)
				return err
			}
		}
		mu.RUnlock()
		buf.Reset()

		// Insert into db
		if _, err := db.Exec(`INSERT INTO chats (author, content, time) VALUES (?, ?, ?)`, msg.Author, msg.Content, msg.Time); err != nil {
			c.Logger().Error(err)
			return err
		}
	}
}

// Render replaces Echo's echo.Context.Render() with templ's templ.Component.Render().
func Render(ctx echo.Context, statusCode int, t templ.Component) error {
	buf := templ.GetBuffer()
	defer templ.ReleaseBuffer(buf)
	if err := t.Render(ctx.Request().Context(), buf); err != nil {
		return err
	}
	return ctx.HTML(statusCode, buf.String())
}

func main() {
	sqlite, err := sql.Open("sqlite3", "var/sqlite.db?cache=shared")
	if err != nil {
		panic(err)
	}
	if _, err := sqlite.Exec(`
		CREATE TABLE IF NOT EXISTS chats (
			author		TEXT NOT NULL,
			content		TEXT NOT NULL,
			time		DATETIME
		)`); err != nil {
		panic(err)
	}
	db = sqlite
	defer db.Close()
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Static("/static", "static")
	e.GET("/ws", ws)
	e.GET("/", func(c echo.Context) error {
		return Render(c, 200, templates.Index())
	})
	e.Logger.Fatal(e.Start(":8080"))
}
