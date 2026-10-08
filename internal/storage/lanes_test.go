package storage

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// laneBoard es el tablero de las pruebas de wb (claude-connect, src/kanban.rs): las dos herramientas deben leerlo igual.
const laneBoard = "---\n\nkanban-plugin: board\n\n---\n\n## lane-1\n\n- [ ] [[note-1]] #t1\n- [x] card-2 #a/b\n\tcont-2\n- [ ] [[dir/note-3|alias-3]] text-3\n\n## lane-2\n\n\n## lane-3\n\n- [ ] card-4\n\n%% kanban:settings\n```\n{\"kanban-plugin\":\"board\"}\n```\n%%\n"

func laneLines() []string { return strings.Split(laneBoard, "\n") }

func TestParseLanesReadsWbsBoard(t *testing.T) {
	b, err := ParseLanes(laneLines())
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, l := range b.Lanes {
		titles = append(titles, l.Title)
	}
	if !slices.Equal(titles, []string{"lane-1", "lane-2", "lane-3"}) {
		t.Fatalf("carriles: %v", titles)
	}
	if !slices.Equal(b.Lanes[0].Cards, []int{9, 10, 12}) || len(b.Lanes[1].Cards) != 0 || !slices.Equal(b.Lanes[2].Cards, []int{19}) {
		t.Fatalf("tarjetas: %+v", b.Lanes)
	}
	if b.LaneOf(11) != -1 || b.LaneOf(10) != 0 || b.LaneOf(19) != 2 {
		t.Fatal("LaneOf: una continuación no es una tarjeta")
	}
	if b.DoneIndex() != -1 || b.LaneIndex("LANE-2") != 1 || b.LaneIndex("x") != -1 {
		t.Fatal("DoneIndex o LaneIndex")
	}
}

func TestParseLanesSkipsCommentsAndNonCards(t *testing.T) {
	src := "## a\n%%\n## hidden\n- [ ] hidden\n%%\n* [ ] star\n  - [ ] nested\n- [/] other\n- [ ]x\n- [ ]\n## Done\n"
	b, err := ParseLanes(strings.Split(src, "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Lanes) != 2 || !slices.Equal(b.Lanes[0].Cards, []int{10}) || b.DoneIndex() != 1 {
		t.Fatalf("%+v", b)
	}
}

func TestParseLanesBounds(t *testing.T) {
	if _, err := ParseLanes(strings.Split(strings.Repeat("## l\n", MaxLanes), "\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseLanes(strings.Split(strings.Repeat("## l\n", MaxLanes+1), "\n")); !errors.Is(err, ErrTooManyLanes) {
		t.Fatalf("%v", err)
	}
	if _, err := ParseLanes([]string{"---", "a: b"}); !errors.Is(err, ErrUnclosedFrontMatter) {
		t.Fatalf("%v", err)
	}
	if b, _ := ParseLanes([]string{"---", "kanban-plugin: board", "---", "## a", "- [ ] c"}); !slices.Equal(b.Lanes[0].Cards, []int{5}) {
		t.Fatal("frontmatter sin línea en blanco después")
	}
}

func TestMoveCardIntoAnEmptyLane(t *testing.T) {
	l := laneLines()
	out, at, err := moveCard(l, 10, 1)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := ParseLanes(out)
	if !slices.Equal(b.Lanes[1].Cards, []int{at}) || out[at-1] != "- [x] card-2 #a/b" || out[at] != "\tcont-2" {
		t.Fatalf("línea %d: %q", at, out)
	}
	if len(b.Lanes[0].Cards) != 2 {
		t.Fatal("el carril de origen pierde la tarjeta")
	}
	a, o := slices.Clone(out), slices.Clone(l)
	slices.Sort(a)
	slices.Sort(o)
	if !slices.Equal(a, o) {
		t.Fatal("solo se reordenan líneas")
	}
	if _, _, err := moveCard(l, 10, 9); err == nil {
		t.Fatal("carril inexistente")
	}
	if _, _, err := moveCard(l, 11, 1); !errors.Is(err, ErrNotACard) {
		t.Fatalf("una continuación no se mueve: %v", err)
	}
}

func TestMoveCardToTheEndOfALaneUpAndDown(t *testing.T) {
	l := laneLines()
	out, at, err := moveCard(l, 9, 2)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := ParseLanes(out)
	if got := b.Lanes[2].Cards; len(got) != 2 || got[1] != at || out[at-1] != "- [ ] [[note-1]] #t1" || out[got[0]-1] != "- [ ] card-4" {
		t.Fatalf("abajo: %v %q", got, out)
	}
	back, at2, err := moveCard(l, 19, 0)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = ParseLanes(back)
	if got := b.Lanes[0].Cards; got[len(got)-1] != at2 || back[at2-1] != "- [ ] card-4" {
		t.Fatalf("arriba: %v %q", got, back)
	}
	same, at3, _ := moveCard(l, 9, 0)
	if at3 != 9 || !slices.Equal(same, l) {
		t.Fatal("a su propio carril no cambia nada")
	}
}

func TestMoveCardKeepsCRLF(t *testing.T) {
	l := strings.Split("## a\r\n- [ ] one\r\n## b\r\n\r\n- [ ] two\r\n", "\n")
	out, at, err := moveCard(l, 2, 1)
	if err != nil || at != 5 || out[4] != "- [ ] one\r" || out[3] != "- [ ] two\r" || out[1] != "## b\r" {
		t.Fatalf("%d %q %v", at, out, err)
	}
}

func TestMoveCardToLaneWritesAndGuards(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "Kanban.md")
	if err := os.WriteFile(p, []byte(laneBoard), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(dir)
	if _, err := s.MoveCardToLane(p, 9, 1, time.Unix(1, 0)); !errors.Is(err, ErrNoteChanged) {
		t.Fatalf("mtime distinto: %v", err)
	}
	at, err := s.MoveCardToLane(p, 9, 1, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.LanesOf(p)
	if err != nil || !slices.Equal(b.Lanes[1].Cards, []int{at}) {
		t.Fatalf("%+v %v", b, err)
	}
}

func TestListNotesSkipsBuildFolders(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"a.md", "node_modules/x/b.md", "target/c.md", "notes/d.md"} {
		p := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	notes, err := New(dir).ListNotes()
	if err != nil || len(notes) != 2 {
		t.Fatalf("%d notas, %v", len(notes), err)
	}
}
