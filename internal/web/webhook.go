package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"minirail/internal/apps"
)

type pushEvent struct {
	Ref        string `json:"ref"`
	After      string `json:"after"`
	Deleted    bool   `json:"deleted"`
	Repository struct {
		HTMLURL string `json:"html_url"`
	} `json:"repository"`
}

func VerifySignature(secret string, body []byte, header string) bool {
	signature, found := strings.CutPrefix(header, "sha256=")
	if !found {
		return false
	}
	given, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(given, mac.Sum(nil))
}

func (s *Server) webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if _, tooLarge := errors.AsType[*http.MaxBytesError](err); tooLarge {
		http.Error(w, "payload too large: press Deploy on the app page", http.StatusRequestEntityTooLarge)
		return
	}
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	if !VerifySignature(s.Config.WebhookSecret, body, r.Header.Get("X-Hub-Signature-256")) {
		log.Printf("webhook: bad signature from %s", r.RemoteAddr)
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}
	switch r.Header.Get("X-GitHub-Event") {
	case "ping":
		w.WriteHeader(http.StatusOK)
		return
	case "push":
	default:
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var event pushEvent
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "invalid push payload", http.StatusBadRequest)
		return
	}
	branch, isBranch := strings.CutPrefix(event.Ref, "refs/heads/")
	if event.Deleted || !isBranch {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !apps.ValidSHA(event.After) {
		http.Error(w, "invalid after sha", http.StatusBadRequest)
		return
	}
	list, err := s.Store.List(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	repo := apps.NormalizeRepo(event.Repository.HTMLURL)
	var matched int
	var errs []error
	for _, app := range list {
		if app.Repo != repo || app.Branch != branch {
			continue
		}
		matched++
		if _, err := s.Builder.Trigger(r.Context(), app, event.After); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", app.Name, err))
		}
	}
	if err := errors.Join(errs...); err != nil {
		fail(w, err)
		return
	}
	if matched == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
