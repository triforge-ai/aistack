package workspace

import "testing"

func TestSaveChatMemoryDefault(t *testing.T) {
	// Unset config defaults to on.
	ws := &Workspace{}
	if !ws.SaveChatMemory() {
		t.Fatal("SaveChatMemory should default to true")
	}

	off := false
	ws.Chat.SaveMemory = &off
	if ws.SaveChatMemory() {
		t.Fatal("SaveChatMemory should honor an explicit false")
	}

	on := true
	ws.Chat.SaveMemory = &on
	if !ws.SaveChatMemory() {
		t.Fatal("SaveChatMemory should honor an explicit true")
	}
}
