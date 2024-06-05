package localserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/kolide/launcher/pkg/traces"
	webrtc "github.com/pion/webrtc/v4"
)

func (ls *localServer) webrtcHandler() http.Handler {
	return http.HandlerFunc(ls.webrtcHandlerFunc)
}

type (
	webrtcConnectionHandler struct {
		conn     *webrtc.PeerConnection
		slogger  *slog.Logger
		shutdown chan struct{}
	}

	webrtcRequest struct {
		SessionDescription string `json:"client_session_description"`
	}

	webrtcResponse struct {
		ClientSDP   string `json:"client_sdp"`
		LauncherSDP string `json:"launcher_sdp"`
	}
)

func (ls *localServer) webrtcHandlerFunc(w http.ResponseWriter, r *http.Request) {
	r, span := traces.StartHttpRequestSpan(r, "path", r.URL.Path)
	defer span.End()

	if r.Body == nil {
		sendClientError(w, span, errors.New("webrtc request body is nil"))
		return
	}

	var body webrtcRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		sendClientError(w, span, fmt.Errorf("error unmarshaling request body: %w", err))
		return
	}

	h, err := ls.newWebrtcHandler(body.SessionDescription)
	if err != nil {
		h.close()
		sendClientError(w, span, fmt.Errorf("error creating webrtc handler: %w", err))
		return
	}

	localSessionDescription, err := h.localDescription()
	if err != nil {
		h.close()
		sendClientError(w, span, fmt.Errorf("error getting webrtc session description: %w", err))
		return
	}

	// Set the conn handler on localserver so we can shut it down
	ls.setWebrtcConn(h)

	// TODO RM: Send localSessionDescription in callback -- for now, just logs
	respBody := webrtcResponse{
		ClientSDP:   body.SessionDescription,
		LauncherSDP: localSessionDescription,
	}
	ls.slogger.Log(r.Context(), slog.LevelInfo,
		"got local session description",
		"description", localSessionDescription,
		"resp", respBody,
	)

	go h.run()
}

func (ls *localServer) newWebrtcHandler(sessionDescriptionRaw string) (*webrtcConnectionHandler, error) {
	conn, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return nil, fmt.Errorf("creating peer connection: %w", err)
	}

	w := &webrtcConnectionHandler{
		conn:     conn,
		slogger:  ls.slogger.With("component", "webrtc_handler"),
		shutdown: make(chan struct{}),
	}

	// Prepare our handlers
	w.conn.OnConnectionStateChange(w.handleWebrtcConnectionStateChange)
	w.conn.OnDataChannel(w.handleDataChannel)

	// Extract and set remote description
	remoteDescription, err := extractRemoteDescription(sessionDescriptionRaw)
	if err != nil {
		return nil, fmt.Errorf("extracting remote description from request: %w", err)
	}
	if err := w.conn.SetRemoteDescription(remoteDescription); err != nil {
		return nil, fmt.Errorf("setting remote description: %w", err)
	}

	// Create local description
	answer, err := w.conn.CreateAnswer(nil)
	if err != nil {
		return nil, fmt.Errorf("creating local description: %w", err)
	}
	if err := w.conn.SetLocalDescription(answer); err != nil {
		return nil, fmt.Errorf("setting local description: %w", err)
	}

	return w, nil
}

func extractRemoteDescription(sessionDescriptionRaw string) (webrtc.SessionDescription, error) {
	descriptionDecoded, err := base64.StdEncoding.DecodeString(sessionDescriptionRaw)
	if err != nil {
		return webrtc.SessionDescription{}, fmt.Errorf("decoding session description: %w", err)
	}

	var remoteDescription webrtc.SessionDescription
	if err := json.Unmarshal(descriptionDecoded, &remoteDescription); err != nil {
		return remoteDescription, fmt.Errorf("unmarshalling session description: %w", err)
	}

	return remoteDescription, nil
}

func (w *webrtcConnectionHandler) handleDataChannel(d *webrtc.DataChannel) {
	d.OnOpen(func() {
		w.slogger.Log(context.TODO(), slog.LevelInfo,
			"data channel opened",
		)
	})

	d.OnMessage(func(msg webrtc.DataChannelMessage) {
		w.slogger.Log(context.TODO(), slog.LevelInfo,
			"received message",
			"message", string(msg.Data),
		)
	})
}

func (w *webrtcConnectionHandler) handleWebrtcConnectionStateChange(s webrtc.PeerConnectionState) {
	w.slogger.Log(context.TODO(), slog.LevelInfo,
		"peer connection state has changed",
		"new_state", s.String(),
	)

	if s == webrtc.PeerConnectionStateFailed || s == webrtc.PeerConnectionStateClosed {
		w.shutdown <- struct{}{}
	}
}

func (w *webrtcConnectionHandler) localDescription() (string, error) {
	descriptionRaw, err := json.Marshal(w.conn.LocalDescription())
	if err != nil {
		return "", fmt.Errorf("marshalling local description: %w", err)
	}

	return base64.StdEncoding.EncodeToString(descriptionRaw), nil
}

func (w *webrtcConnectionHandler) run() {
	<-w.shutdown
	w.close()
}

func (w *webrtcConnectionHandler) close() {
	w.slogger.Log(context.TODO(), slog.LevelInfo,
		"shutting down",
	)
	w.conn.Close()
	// TODO RM: This requires a refactor to be able to set ls.webrtcConn to nil after close
}
