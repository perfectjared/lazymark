package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/MathiasDrizzy/lazymark/internal/config"
)

// newLaneModel abre la app con --board sobre un tablero de carriles recién escrito.
func newLaneModel(t *testing.T, board string) (*AppModel, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("AppData", filepath.Join(home, "AppData"))
	dir := t.TempDir()
	p := filepath.Join(dir, "Kanban.md")
	if err := os.WriteFile(p, []byte(board), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "other.md"), []byte("- [ ] o\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig(dir)
	cfg.Language = "es"
	cfg.Board = p
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	m.c.clip.Reader = noRealClipboard{}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 35})
	return m, p
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestLaneBoardOpensMovesAndToggles: --board abre en el tablero de la nota, con sus carriles como columnas y solo sus tarjetas; L mueve
// el bloque de la tarjeta al carril de la derecha y el cursor la sigue; Espacio la marca sin moverla.
func TestLaneBoardOpensMovesAndToggles(t *testing.T) {
	m, p := newLaneModel(t, "## To do\n\n- [ ] a\n- [ ] b\n\n## Doing\n\n## Done\n")
	if !m.kanbanOn || m.c.board.NumCols() != 3 || m.c.board.TotalCards() != 2 {
		t.Fatalf("abre en el tablero de carriles: on=%v cols=%d cards=%d", m.kanbanOn, m.c.board.NumCols(), m.c.board.TotalCards())
	}
	press(m, "L")
	if got := read(t, p); got != "## To do\n\n- [ ] b\n\n## Doing\n\n- [ ] a\n## Done\n" {
		t.Fatalf("mover: %q", got)
	}
	if m.kanban.col != 1 || m.kanban.current() == nil || m.kanban.current().Task.Text != "a" {
		t.Fatalf("el cursor sigue a la tarjeta: col %d", m.kanban.col)
	}
	press(m, "space")
	if got := read(t, p); !strings.Contains(got, "## Doing\n\n- [x] a") {
		t.Fatalf("marcar no mueve: %q", got)
	}
	if m.kanban.col != 1 {
		t.Fatal("Espacio no cambia de carril")
	}
}
