package runner

import "testing"

func TestTailLines(t *testing.T) {
	tests := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{name: "n<=0 returns empty", in: "a\nb\n", n: 0, want: ""},
		{name: "trims trailing blanks", in: "a\nb\n\n\n", n: 2, want: "a\nb"},
		{name: "returns whole when fewer lines than n", in: "a\nb\n", n: 5, want: "a\nb"},
		{name: "returns last n lines", in: "1\n2\n3\n4\n5\n", n: 2, want: "4\n5"},
		{name: "handles windows newlines", in: "1\r\n2\r\n3\r\n", n: 2, want: "2\n3"},
		{name: "handles lone CR", in: "1\r2\r3\r", n: 2, want: "2\n3"},
		{name: "ignores trailing whitespace-only lines", in: "1\n2\n \n\t\n", n: 1, want: "2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TailLines(tt.in, tt.n)
			if got != tt.want {
				t.Fatalf("TailLines(n=%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}
