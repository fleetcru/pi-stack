package server

import (
	"net/http"
	"net/url"
	"testing"
)

func TestValidateWorkerURLRejectsPrivateHostWithoutAllowlist(t *testing.T) {
	s := newTestServer(t, "")
	s.cfg.AllowedWorkerHosts = nil
	if _, err := s.validateWorkerURL("http://127.0.0.1:3141"); err == nil {
		t.Fatal("private worker host was accepted without an allowlist")
	}
}

func TestWorkerRedirectRevalidatesDestination(t *testing.T) {
	s := newTestServer(t, "")
	requestURL, err := url.Parse("http://localhost:3141/healthz")
	if err != nil {
		t.Fatal(err)
	}
	request := &http.Request{URL: requestURL}
	if err := s.httpClient.CheckRedirect(request, []*http.Request{{URL: &url.URL{Scheme: "http", Host: "127.0.0.1:3141"}}}); err == nil {
		t.Fatal("redirect to a host outside the worker allowlist was accepted")
	}
}

func TestValidateWorkerURLRejectsCredentials(t *testing.T) {
	s := newTestServer(t, "")
	if _, err := s.validateWorkerURL("http://user:secret@127.0.0.1:3141"); err == nil {
		t.Fatal("worker URL credentials were accepted")
	}
}
