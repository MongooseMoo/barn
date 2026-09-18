package main

import (
	"reflect"
	"testing"
)

func TestLoginRespondsToSplitPromptsWithoutSendingCredentialsEarly(t *testing.T) {
	p := newPromptBuffer()
	done := make(chan struct{})
	var sent []int
	var phases []string
	n, err := loginCommands([]string{"PROXY TCP4 test", "account", "secret", "look"}, "MOTD", p, done, func(i int, text string) error {
		sent = append(sent, i)
		switch i {
		case 0:
			p.feed("Enter your user")
			p.feed("name or email:")
		case 1:
			p.feed("Pass")
			p.feed("word")
		case 2:
			p.feed("Welcome!\nMO")
			p.feed("TD")
		}
		return nil
	}, func(phase string) { phases = append(phases, phase) })
	if err != nil || n != 3 || !reflect.DeepEqual(sent, []int{0, 1, 2}) || !reflect.DeepEqual(phases, []string{"username", "password", "login_ready"}) {
		t.Fatalf("n=%d err=%v sent=%v phases=%v", n, err, sent, phases)
	}
}

func TestLoginStopsOnClosedConnectionBeforeSendingPassword(t *testing.T) {
	p := newPromptBuffer()
	p.feed("Enter your username or email:")
	done := make(chan struct{})
	calls := 0
	_, err := loginCommands([]string{"account", "secret"}, "MOTD", p, done, func(int, string) error {
		calls++
		close(done)
		return nil
	}, func(string) {})
	if err == nil || calls != 1 {
		t.Fatalf("err=%v sends=%d", err, calls)
	}
}
