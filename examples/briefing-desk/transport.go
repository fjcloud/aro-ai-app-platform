package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func checkedURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", errors.New("invalid service URL")
	}
	return u.String(), nil
}
func readCredential(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil || len(b) > 16384 || strings.TrimSpace(string(b)) == "" {
		return "", errors.New("service credential unavailable")
	}
	return strings.TrimSpace(string(b)), nil
}
func serviceClient(caFile string) (*http.Client, error) {
	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}
	if caFile != "" {
		pem, err := os.ReadFile(caFile)
		if err != nil || !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("service CA unavailable")
		}
	}
	return &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("service redirects refused") }}, nil
}
