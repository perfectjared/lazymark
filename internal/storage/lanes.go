package storage

// Tableros de carriles: una nota con la forma del plugin Kanban de Obsidian (`## Carril` y, debajo, tarjetas `- [ ] texto`), leída y
// editada en su lugar. Las reglas son las de wb (claude-connect, src/kanban.rs), para que las dos herramientas lean igual el mismo
// archivo: el frontmatter (`---` … `---`) se salta, cada encabezado `## ` es un carril, los bloques `%%` se saltan, una tarjeta es
// `- [ ]`, `- [x]` o `- [X]` en el margen, y su bloque es su línea más las sangradas que la siguen hasta la primera en blanco.

import (
	"strings"
	"time"

	"github.com/MathiasDrizzy/lazymark/internal/i18n"
	"github.com/MathiasDrizzy/lazymark/internal/safeio"
)

// MaxLanes es el máximo de carriles de un tablero (el mismo que wb).
const MaxLanes = 32

// ErrTooManyLanes es el error de un tablero con más de MaxLanes carriles.
var ErrTooManyLanes = i18n.NewError("el tablero tiene demasiados carriles", "the board has too many lanes")

// ErrUnclosedFrontMatter es el error de un tablero cuyo frontmatter abre con `---` y no cierra.
var ErrUnclosedFrontMatter = i18n.NewError("el frontmatter no se cierra", "the front matter is not closed")

// ErrNotACard es el error de mover una línea que no es una tarjeta del tablero.
var ErrNotACard = i18n.NewError("la línea no es una tarjeta del tablero", "the line is not a card on the board")

// Lane es un carril: su título, la línea de su encabezado y las de sus tarjetas (desde 1).
type Lane struct {
	Title   string
	Heading int
	Cards   []int
	insert  int // índice (desde 0) donde va una tarjeta movida aquí: tras la última tarjeta, o tras el encabezado y su línea en blanco
}

// LaneBoard son los carriles de una nota, en orden.
type LaneBoard struct {
	Lanes []Lane
}

// LaneOf devuelve el carril de la tarjeta de la línea line (desde 1), o -1 si no es una tarjeta.
func (b LaneBoard) LaneOf(line int) int {
	for i, l := range b.Lanes {
		for _, c := range l.Cards {
			if c == line {
				return i
			}
		}
	}
	return -1
}

// DoneIndex devuelve el carril que se llama "done" (sin distinguir mayúsculas), o -1. Solo cambia cómo se ve: mover o marcar no
// depende de él.
func (b LaneBoard) DoneIndex() int {
	for i, l := range b.Lanes {
		if strings.EqualFold(l.Title, "done") {
			return i
		}
	}
	return -1
}

// LaneIndex devuelve el carril con ese título (sin distinguir mayúsculas), o -1.
func (b LaneBoard) LaneIndex(title string) int {
	for i, l := range b.Lanes {
		if strings.EqualFold(l.Title, strings.TrimSpace(title)) {
			return i
		}
	}
	return -1
}

// isCardLine dice si l es una tarjeta: `- [ ]`, `- [x]` o `- [X]` en el margen, seguido de un espacio o del fin de la línea.
func isCardLine(l string) bool {
	rest, ok := strings.CutPrefix(l, "- [")
	if !ok || rest == "" || !strings.ContainsRune(" xX", rune(rest[0])) {
		return false
	}
	after, ok := strings.CutPrefix(rest[1:], "]")
	return ok && (after == "" || after[0] == ' ')
}

// isContinuation dice si l sigue a una tarjeta: sangrada y no en blanco.
func isContinuation(l string) bool {
	return (strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t")) && strings.TrimSpace(l) != ""
}

// cardEnd devuelve el índice (desde 0) de la última línea del bloque de la tarjeta de lines[i].
func cardEnd(lines []string, i int) int {
	end := i
	for end+1 < len(lines) && isContinuation(strings.TrimSuffix(lines[end+1], "\r")) {
		end++
	}
	return end
}

// ParseLanes lee los carriles de las líneas de una nota (con su \r, si lo tienen).
func ParseLanes(lines []string) (LaneBoard, error) {
	i := 0
	if len(lines) > 0 && strings.TrimRight(lines[0], " \t\r") == "---" {
		i = -1
		for j := 1; j < len(lines); j++ {
			if strings.TrimRight(lines[j], " \t\r") == "---" {
				i = j + 1
				break
			}
		}
		if i < 0 {
			return LaneBoard{}, ErrUnclosedFrontMatter
		}
	}
	var b LaneBoard
	comment := false
	for i < len(lines) {
		l := strings.TrimSuffix(lines[i], "\r")
		switch {
		case comment:
			comment = !strings.Contains(l, "%%")
		case strings.HasPrefix(l, "%%"):
			comment = !strings.Contains(l[2:], "%%")
		case strings.HasPrefix(l, "## "):
			if len(b.Lanes) == MaxLanes {
				return LaneBoard{}, ErrTooManyLanes
			}
			insert := i + 1
			if i+1 < len(lines) && strings.TrimSpace(lines[i+1]) == "" {
				insert++
			}
			b.Lanes = append(b.Lanes, Lane{Title: strings.TrimSpace(l[3:]), Heading: i + 1, insert: insert})
		case len(b.Lanes) > 0 && isCardLine(l):
			lane := &b.Lanes[len(b.Lanes)-1]
			end := cardEnd(lines, i)
			lane.Cards = append(lane.Cards, i+1)
			lane.insert = end + 1
			i = end + 1
			continue
		}
		i++
	}
	return b, nil
}

// moveCard devuelve lines con la tarjeta de la línea line (desde 1), con su bloque, al final del carril lane, y la línea nueva de la
// tarjeta. Solo cambia de lugar líneas: la casilla y el texto no se tocan (como move_card de wb).
func moveCard(lines []string, line, lane int) ([]string, int, error) {
	b, err := ParseLanes(lines)
	if err != nil {
		return nil, 0, err
	}
	if lane < 0 || lane >= len(b.Lanes) {
		return nil, 0, i18n.Errorf("el carril %d no existe (hay %d)", "lane %d does not exist (there are %d)", lane, len(b.Lanes))
	}
	from := b.LaneOf(line)
	if from < 0 {
		return nil, 0, i18n.Errorf("%w: línea %d", "%w: line %d", ErrNotACard, line)
	}
	if from == lane {
		return lines, line, nil
	}
	start := line - 1
	end := cardEnd(lines, start)
	n := end - start + 1
	at := b.Lanes[lane].insert
	if at > start {
		at -= n
	}
	block := append([]string(nil), lines[start:end+1]...)
	rest := append(append([]string(nil), lines[:start]...), lines[end+1:]...)
	out := append(append(append(make([]string, 0, len(lines)), rest[:at]...), block...), rest[at:]...)
	return out, at + 1, nil
}

// LanesOf lee los carriles de la nota notePath.
func (s *Storage) LanesOf(notePath string) (LaneBoard, error) {
	notePath, err := s.ResolveNote(notePath)
	if err != nil {
		return LaneBoard{}, err
	}
	data, err := safeio.ReadRegular(notePath, MaxNoteBytes)
	if err != nil {
		return LaneBoard{}, err
	}
	return ParseLanes(strings.Split(string(data), "\n"))
}

// MoveCardToLane mueve la tarjeta de la línea line (desde 1) de la nota al final del carril lane y devuelve su línea nueva. Si expected
// no es cero y la nota cambió en disco desde que se leyó, no escribe y devuelve ErrNoteChanged.
func (s *Storage) MoveCardToLane(notePath string, line, lane int, expected time.Time) (int, error) {
	notePath, err := s.ResolveNote(notePath)
	if err != nil {
		return 0, err
	}
	newLine := 0
	err = rewriteLines(notePath, expected, func(lines []string) ([]string, error) {
		out, at, err := moveCard(lines, line, lane)
		newLine = at
		return out, err
	})
	if err != nil {
		return 0, err
	}
	return newLine, nil
}
