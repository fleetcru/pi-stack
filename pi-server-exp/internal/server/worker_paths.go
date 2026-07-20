package server

import "strings"

type workerPath struct {
	id   string
	rest string
}

func splitWorkerPath(path string) workerPath {
	tail := strings.TrimPrefix(path, "/v1/workers/")
	parts := strings.SplitN(tail, "/", 2)
	wp := workerPath{id: parts[0]}
	if len(parts) == 2 {
		wp.rest = parts[1]
	}
	return wp
}
