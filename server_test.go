package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hacel/htmxchat/templates"
)

func TestParseIncomingMessage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		payload string
		want    string
		wantErr bool
	}{
		{name: "valid", payload: `{"chat_message":"hello"}`, want: "hello"},
		{name: "ignores server fields", payload: `{"chat_message":"hello","Author":"spoofed"}`, want: "hello"},
		{name: "preserves whitespace", payload: `{"chat_message":" hello "}`, want: " hello "},
		{name: "empty", payload: `{"chat_message":"  "}`, wantErr: true},
		{name: "invalid JSON", payload: `{`, wantErr: true},
		{name: "too long", payload: `{"chat_message":"` + strings.Repeat("a", maxMessageRunes+1) + `"}`, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseIncomingMessage([]byte(test.payload))
			if (err != nil) != test.wantErr {
				t.Fatalf("parseIncomingMessage() error = %v, wantErr %v", err, test.wantErr)
			}
			if got != test.want {
				t.Errorf("parseIncomingMessage() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestColorForAlwaysReturnsPaletteColor(t *testing.T) {
	t.Parallel()
	palette := make(map[string]bool, len(colors))
	for _, color := range colors {
		palette[color] = true
	}
	for i := 0; i < 1000; i++ {
		color := colorFor(string(rune(i)))
		if !palette[color] {
			t.Fatalf("colorFor(%d) returned unknown color %q", i, color)
		}
	}
}

func TestRecentMessagesReturnsNewestInChronologicalOrder(t *testing.T) {
	t.Parallel()
	database, err := openDatabase(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	ctx := context.Background()
	for i, content := range []string{"first", "second", "third"} {
		message := templates.Message{
			Author:  "author",
			Content: content,
			Time:    time.Date(2026, 1, 1, 0, i, 0, 0, time.UTC),
		}
		if err := saveMessage(ctx, database, message); err != nil {
			t.Fatal(err)
		}
	}

	messages, err := recentMessages(ctx, database, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Content != "second" || messages[1].Content != "third" {
		t.Fatalf("recentMessages() = %#v", messages)
	}
}

func TestHTTPRoutes(t *testing.T) {
	t.Parallel()
	database, err := openDatabase(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	e := newHTTPServer(newChatServer(database))

	for _, test := range []struct {
		path       string
		statusCode int
		contains   string
	}{
		{path: "/", statusCode: http.StatusOK, contains: `<form id="message_form" ws-send`},
		{path: "/static/main.css", statusCode: http.StatusOK, contains: "tailwindcss"},
		{path: "/static/app.js", statusCode: http.StatusOK, contains: "htmx:wsOpen"},
		{path: "/static/htmx.min.js", statusCode: http.StatusOK, contains: "var htmx=function"},
		{path: "/static/ws.min.js", statusCode: http.StatusOK, contains: "htmx.defineExtension"},
		{path: "/healthz", statusCode: http.StatusNoContent},
	} {
		t.Run(test.path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			recorder := httptest.NewRecorder()
			e.ServeHTTP(recorder, request)
			if recorder.Code != test.statusCode {
				t.Fatalf("GET %s status = %d, want %d", test.path, recorder.Code, test.statusCode)
			}
			if test.contains != "" && !strings.Contains(recorder.Body.String(), test.contains) {
				t.Errorf("GET %s body does not contain %q", test.path, test.contains)
			}
		})
	}

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, request)
	policy := recorder.Header().Get("Content-Security-Policy")
	if policy == "" || strings.Contains(policy, "unsafe-inline") {
		t.Errorf("Content-Security-Policy = %q", policy)
	}
}
