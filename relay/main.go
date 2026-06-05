// Command cf-email-relay is a localhost SMTP server that accepts mail from
// services that can't speak HTTP and relays each message to a Cloudflare Worker
// (the worker/ in this repo) over HTTPS, which sends it via Cloudflare Email
// Sending.
package main

import "log"

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := run(cfg); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}
