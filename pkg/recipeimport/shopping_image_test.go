package recipeimport

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestDecodeImagePayload(t *testing.T) {
	raw := []byte{0xff, 0xd8, 0xff, 0xd9}
	encoded := base64.StdEncoding.EncodeToString(raw)

	data, mime, err := decodeImagePayload(encoded, "image/jpeg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mime != "image/jpeg" {
		t.Fatalf("mime = %q", mime)
	}
	if string(data) != string(raw) {
		t.Fatalf("decoded bytes mismatch")
	}

	dataURL := "data:image/png;base64," + encoded
	_, mime, err = decodeImagePayload(dataURL, "image/png")
	if err != nil {
		t.Fatalf("data URL: %v", err)
	}
	if mime != "image/png" {
		t.Fatalf("png mime = %q", mime)
	}

	_, _, err = decodeImagePayload(encoded, "application/pdf")
	if !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("expected invalid image for pdf, got %v", err)
	}

	_, _, err = decodeImagePayload("", "image/jpeg")
	if !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("expected invalid image for empty payload, got %v", err)
	}
}

func TestDecodeImagePayloadNormalizesJPG(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("x"))
	_, mime, err := decodeImagePayload(encoded, "image/jpg")
	if err != nil {
		t.Fatal(err)
	}
	if mime != "image/jpeg" {
		t.Fatalf("jpg should normalize to jpeg, got %q", mime)
	}
}

func TestShoppingImagePromptMentionsGroceries(t *testing.T) {
	prompt := shoppingImagePrompt()
	if !strings.Contains(prompt, "handwritten") {
		t.Fatal("prompt should mention handwritten lists")
	}
}
