package main

import (
	"testing"
)

func TestLegalPlacementsAndSevenBag(t *testing.T) {
	expected := map[string]int{"I": 17, "O": 9, "T": 34, "S": 17, "Z": 17, "J": 34, "L": 34}
	for name, n := range expected {
		g := newGame(42)
		g.Piece = name
		moves := g.moves()
		if len(moves) != n {
			t.Fatalf("%s: %d placements, want %d", name, len(moves), n)
		}
		seen := map[string]bool{}
		for _, m := range moves {
			if seen[m.ID] {
				t.Fatal("duplicate move")
			}
			seen[m.ID] = true
			cells := map[cell]bool{}
			for _, c := range m.Cells {
				if c.X < 0 || c.X >= width || c.Y < 0 || c.Y >= height || cells[c] {
					t.Fatal("invalid tetromino")
				}
				cells[c] = true
			}
			if m.Features.HeightSum != 4+m.Features.Holes {
				t.Fatal("floor column heights must count four blocks plus enclosed holes")
			}
		}
	}
	g := newGame(42)
	seen := map[string]bool{}
	for i := 0; i < 7; i++ {
		if seen[g.Piece] {
			t.Fatal("seven-bag repeated a piece")
		}
		seen[g.Piece] = true
		g.advance()
	}
}
func TestClearFourRowsAndScore(t *testing.T) {
	g := newGame(1)
	g.Piece = "I"
	for y := 16; y < 20; y++ {
		for x := 0; x < 9; x++ {
			g.Board[y][x] = 1
		}
	}
	var chosen *move
	for _, m := range g.moves() {
		if m.Features.Lines == 4 {
			m := m
			chosen = &m
			break
		}
	}
	if chosen == nil {
		t.Fatal("Tetris placement missing")
	}
	g.place(*chosen)
	if g.Lines != 4 || g.Score != 800 || g.Board != (board{}) || g.Turn != 1 {
		t.Fatal("four-row clear corrupted state")
	}
}
func TestSeededBaselineLongRun(t *testing.T) {
	a, b := newGame(42), newGame(42)
	for i := 0; i < 300 && !a.Over; i++ {
		am, bm := a.moves(), b.moves()
		if len(am) != len(bm) || a.Piece != b.Piece || a.Board != b.Board {
			t.Fatal("seed is not reproducible")
		}
		a.place(heuristic(am))
		b.place(heuristic(bm))
	}
	if a.Turn < 100 || a.Lines < 20 {
		t.Fatalf("baseline unexpectedly weak: %d turns, %d lines", a.Turn, a.Lines)
	}
}
func TestTopOutAndHoles(t *testing.T) {
	g := newGame(42)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			g.Board[y][x] = 1
		}
	}
	if len(g.moves()) != 0 {
		t.Fatal("full board has moves")
	}
	b := board{}
	b[15][3] = 1
	f := measure(b)
	if f.Holes != 4 || f.MaxHeight != 5 || f.HeightSum != 5 {
		t.Fatalf("bad holes/height: %+v", f)
	}
}
