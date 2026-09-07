package server

import (
	"context"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

type availableServerModel struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
}

const availableModelsCacheTTL = 5 * time.Minute

func (s *Server) listAvailableModels(w http.ResponseWriter, r *http.Request) {
	s.availableModelsMu.Lock()
	defer s.availableModelsMu.Unlock()
	if len(s.availableModels) > 0 && time.Since(s.availableModelsAt) < availableModelsCacheTTL {
		writeJSON(w, http.StatusOK, map[string]any{"models": s.availableModels})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.RequestTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.cfg.PiBinary, "--list-models")
	cmd.Dir = s.cfg.CWD
	applyProcessAttrs(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if len(detail) > 512 {
			detail = detail[:512]
		}
		s.logger.Warn("could not list Pi models", "error", err, "output", detail)
		writeErrorCode(w, r, http.StatusBadGateway, CodeBadGateway, "could not list Pi models")
		return
	}
	models := parseAvailableModels(string(output))
	if len(models) == 0 {
		s.logger.Warn("Pi returned no available models")
		writeErrorCode(w, r, http.StatusBadGateway, CodeBadGateway, "Pi returned no available models")
		return
	}
	s.availableModels = models
	s.availableModelsAt = time.Now()
	writeJSON(w, http.StatusOK, map[string]any{"models": models})
}

func parseAvailableModels(output string) []availableServerModel {
	lines := strings.Split(output, "\n")
	models := make([]availableServerModel, 0, len(lines))
	seen := make(map[string]struct{})
	inTable := false
	for _, line := range lines {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 2 && strings.EqualFold(fields[0], "provider") && strings.EqualFold(fields[1], "model") {
			inTable = true
			continue
		}
		if !inTable || len(fields) < 2 {
			continue
		}
		provider, id := fields[0], fields[1]
		key := provider + "\x00" + id
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		name := id
		if slash := strings.LastIndex(id, "/"); slash >= 0 && slash+1 < len(id) {
			name = id[slash+1:]
		}
		models = append(models, availableServerModel{Provider: provider, ID: id, Name: name})
	}
	return models
}
