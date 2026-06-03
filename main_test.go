package main

import "testing"

func TestParseCallback(t *testing.T) {
	chatID, userID, answer, ok := parseCallback("captcha:-100:42:17")
	if !ok {
		t.Fatal("expected callback to parse")
	}
	if chatID != -100 || userID != 42 || answer != 17 {
		t.Fatalf("unexpected parsed values: chat=%d user=%d answer=%d", chatID, userID, answer)
	}
}

func TestParseCallbackRejectsInvalidData(t *testing.T) {
	if _, _, _, ok := parseCallback("other:-100:42:17"); ok {
		t.Fatal("expected invalid prefix to be rejected")
	}
	if _, _, _, ok := parseCallback("captcha:-100:bad:17"); ok {
		t.Fatal("expected invalid user id to be rejected")
	}
}
