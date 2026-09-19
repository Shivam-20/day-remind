package web

import (
	"reflect"
	"testing"
)

func TestParseTelegramCommand(t *testing.T) {
	cases := []struct {
		in   string
		cmd  string
		args []string
	}{
		{"/add pay bill", "/add", []string{"pay", "bill"}},
		{"/list@DayRemindBot today", "/list", []string{"today"}},
		{"  /done  abc ", "/done", []string{"abc"}},
		{"/HELP", "/help", nil},
		{"", "", nil},
		{"   ", "", nil},
	}
	for _, c := range cases {
		cmd, args := parseTelegramCommand(c.in)
		if cmd != c.cmd {
			t.Fatalf("parseTelegramCommand(%q) cmd = %q, want %q", c.in, cmd, c.cmd)
		}
		if !reflect.DeepEqual(args, c.args) {
			t.Fatalf("parseTelegramCommand(%q) args = %v, want %v", c.in, args, c.args)
		}
	}
}
