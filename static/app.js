(() => {
  const room = document.getElementById("chat_room");
  const form = document.getElementById("message_form");
  const input = document.getElementById("message_input");
  const status = document.getElementById("connection_status");

  if (!room || !form || !input || !status) {
    return;
  }

  const reconnectingCloseCodes = new Set([1006, 1011, 1012, 1013]);
  let shouldStickToBottom = true;

  const isNearBottom = () =>
    room.scrollHeight - room.scrollTop - room.clientHeight < 80;

  const setConnectionStatus = (state, label) => {
    status.dataset.state = state;
    status.textContent = label;
  };

  const localizeTimes = () => {
    room.querySelectorAll("time[data-local-time]").forEach((element) => {
      const date = new Date(element.dateTime);
      if (Number.isNaN(date.getTime())) {
        return;
      }

      const includeDate = Date.now() - date.getTime() >= 24 * 60 * 60 * 1000;
      element.textContent = new Intl.DateTimeFormat(undefined, {
        ...(includeDate && {
          year: "numeric",
          month: "2-digit",
          day: "2-digit",
        }),
        hour: "numeric",
        minute: "2-digit",
      }).format(date);
      element.title = date.toLocaleString();
      element.removeAttribute("data-local-time");
    });
  };

  room.addEventListener(
    "scroll",
    () => {
      shouldStickToBottom = isNearBottom();
    },
    { passive: true },
  );

  document.body.addEventListener("htmx:wsBeforeMessage", () => {
    shouldStickToBottom = isNearBottom();
  });

  document.body.addEventListener("htmx:wsAfterMessage", () => {
    localizeTimes();
    if (shouldStickToBottom) {
      requestAnimationFrame(() => {
        room.scrollTop = room.scrollHeight;
      });
    }
  });

  document.body.addEventListener("htmx:wsConnecting", () => {
    setConnectionStatus("connecting", "Connecting…");
  });

  document.body.addEventListener("htmx:wsOpen", () => {
    setConnectionStatus("connected", "Connected");
  });

  document.body.addEventListener("htmx:wsClose", (event) => {
    const code = event.detail?.event?.code;
    if (reconnectingCloseCodes.has(code)) {
      setConnectionStatus("connecting", "Reconnecting…");
    } else {
      setConnectionStatus("disconnected", "Disconnected");
    }
  });

  document.body.addEventListener("htmx:wsError", () => {
    setConnectionStatus("connecting", "Connection interrupted…");
  });

  form.addEventListener("htmx:wsAfterSend", () => {
    input.value = "";
    input.focus({ preventScroll: true });
  });

  if (window.matchMedia("(pointer: fine)").matches) {
    input.focus({ preventScroll: true });
  }
})();
