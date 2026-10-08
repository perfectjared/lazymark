package ops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MathiasDrizzy/lazymark/internal/i18n"
	"github.com/MathiasDrizzy/lazymark/internal/storage"
)

// TestNoteIDOfDotDotNames (ORD-018, segunda opinión): una nota o carpeta cuyo nombre empieza con ".." dentro del vault sigue teniendo como id su ruta relativa.
func TestNoteIDOfDotDotNames(t *testing.T) {
	dir := t.TempDir()
	s := &Service{Store: storage.New(dir)}
	if got := s.noteID(filepath.Join(dir, "..oculto.md")); got != "..oculto.md" {
		t.Errorf("id = %q", got)
	}
	if got := s.noteID(filepath.Join(dir, "..d", "n.md")); got != "..d/n.md" {
		t.Errorf("id = %q", got)
	}
	if got := s.noteID(filepath.Join(filepath.Dir(dir), "fuera.md")); strings.HasPrefix(got, "..") {
		t.Errorf("fuera de la carpeta no se hace pasar por relativa: %q", got)
	}
}

// TestEmptyNoteNameIsUsageInEveryLanguage (ORD-019 rev 2 / L17): un título que no deja nombre (`///`) es un error de uso (código 2) igual en español que en inglés:
// el código no decide por el texto traducido del error, sino por un error centinela.
func TestEmptyNoteNameIsUsageInEveryLanguage(t *testing.T) {
	dir := t.TempDir()
	s := &Service{Store: storage.New(dir)}
	defer i18n.SetLanguage("es")
	for _, lang := range []string{"es", "en", "fr"} {
		i18n.SetLanguage(lang)
		for _, title := range []string{"///", "...", "   /   "} {
			_, err := s.NewNote(title, "", true)
			if err == nil {
				t.Fatalf("%s: %q no deja nombre: debe fallar", lang, title)
			}
			if code := Code(err); code != ExitUsage {
				t.Errorf("%s: %q: código %d (%v), se esperaba %d", lang, title, code, err, ExitUsage)
			}
		}
	}
}

// TestLaneBoardAndMoveCard: un tablero de carriles se lee con sus carriles como columnas, y mover una tarjeta cambia su carril, no su id.
func TestLaneBoardAndMoveCard(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "Kanban.md")
	if err := os.WriteFile(p, []byte("---\nkanban-plugin: board\n---\n\n## To do\n\n- [ ] a\n- [ ] b\n\n## Doing\n\n## Done\n\n- [x] c\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Service{Store: storage.New(dir), Cols: storage.DefaultColumns, Titles: []string{"todo", "doing", "done"}}
	b, err := s.LaneBoard("Kanban.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Columns) != 3 || b.Columns[0].ID != "To do" || len(b.Columns[0].Cards) != 2 || b.Columns[2].Cards[0].Column != "Done" {
		t.Fatalf("%+v", b)
	}
	id := b.Columns[0].Cards[0].ID
	moved, err := s.MoveCard(id, "doing")
	if err != nil {
		t.Fatal(err)
	}
	if moved.ID != id || moved.Column != "Doing" || moved.Text != "a" || moved.Done {
		t.Fatalf("%+v", moved)
	}
	if b, _ = s.LaneBoard("Kanban.md"); len(b.Columns[1].Cards) != 1 || len(b.Columns[0].Cards) != 1 {
		t.Fatalf("%+v", b)
	}
	if _, err := s.MoveCard(id, "nope"); Code(err) != ExitUsage {
		t.Fatalf("carril inexistente: %v", err)
	}
}
