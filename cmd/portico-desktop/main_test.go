package main

import "testing"

func TestRequireLoopbackURL(t *testing.T) {
	for _, value := range []string{"http://127.0.0.1:32500", "http://[::1]:32500", "http://localhost:32500"} {
		if err := requireLoopbackURL(value); err != nil {
			t.Fatalf("requireLoopbackURL(%q): %v", value, err)
		}
	}
	for _, value := range []string{"https://127.0.0.1:32500", "http://192.168.1.2:32500", "https://web.getportico.tv"} {
		if err := requireLoopbackURL(value); err == nil {
			t.Fatalf("requireLoopbackURL(%q) unexpectedly succeeded", value)
		}
	}
}
