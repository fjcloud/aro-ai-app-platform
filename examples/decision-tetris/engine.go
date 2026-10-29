package main

import (
	"fmt"
	"math"
	"math/rand"
)

const width, height = 10, 20

type board [height][width]uint8
type cell struct {
	X int `json:"x"`
	Y int `json:"y"`
}
type features struct {
	Lines      int `json:"lines"`
	Holes      int `json:"holes"`
	AddedHoles int `json:"added_holes"`
	HeightSum  int `json:"height_sum"`
	MaxHeight  int `json:"max_height"`
	Bumpiness  int `json:"bumpiness"`
}
type move struct {
	ID       string   `json:"id"`
	Rotation int      `json:"rotation"`
	Column   int      `json:"column"`
	Cells    [4]cell  `json:"cells"`
	Features features `json:"features"`
}

var shapes = map[string][4]cell{
	"I": {{0, 0}, {1, 0}, {2, 0}, {3, 0}}, "O": {{0, 0}, {1, 0}, {0, 1}, {1, 1}},
	"T": {{1, 0}, {0, 1}, {1, 1}, {2, 1}}, "S": {{1, 0}, {2, 0}, {0, 1}, {1, 1}},
	"Z": {{0, 0}, {1, 0}, {1, 1}, {2, 1}}, "J": {{0, 0}, {0, 1}, {1, 1}, {2, 1}},
	"L": {{2, 0}, {0, 1}, {1, 1}, {2, 1}},
}
var pieceNames = []string{"I", "O", "T", "S", "Z", "J", "L"}

type game struct {
	Board board    `json:"board"`
	Piece string   `json:"piece"`
	Next  []string `json:"next"`
	Seed  int64    `json:"seed"`
	Turn  int      `json:"turn"`
	Lines int      `json:"lines"`
	Score int      `json:"score"`
	Over  bool     `json:"over"`
	rng   *rand.Rand
	queue []string
}

func newGame(seed int64) *game {
	g := &game{Seed: seed, rng: rand.New(rand.NewSource(seed))}
	g.advance()
	return g
}
func (g *game) advance() {
	for len(g.queue) < 8 {
		bag := append([]string(nil), pieceNames...)
		g.rng.Shuffle(len(bag), func(i, j int) { bag[i], bag[j] = bag[j], bag[i] })
		g.queue = append(g.queue, bag...)
	}
	g.Piece = g.queue[0]
	g.queue = g.queue[1:]
	g.Next = append([]string(nil), g.queue[:5]...)
	g.Over = len(g.moves()) == 0
}
func rotate(s [4]cell) [4]cell {
	r := [4]cell{}
	mx, my := 100, 100
	for i, c := range s {
		r[i] = cell{-c.Y, c.X}
		mx = min(mx, r[i].X)
		my = min(my, r[i].Y)
	}
	for i := range r {
		r[i].X -= mx
		r[i].Y -= my
	}
	return r
}
func fits(b board, s [4]cell, x, y int) bool {
	for _, c := range s {
		xx, yy := c.X+x, c.Y+y
		if xx < 0 || xx >= width || yy < 0 || yy >= height || b[yy][xx] != 0 {
			return false
		}
	}
	return true
}
func clearRows(b board) (board, int) {
	out := board{}
	dst := height - 1
	n := 0
	for y := height - 1; y >= 0; y-- {
		full := true
		for _, v := range b[y] {
			if v == 0 {
				full = false
				break
			}
		}
		if full {
			n++
			continue
		}
		out[dst] = b[y]
		dst--
	}
	return out, n
}
func measure(b board) features {
	f := features{}
	hs := [width]int{}
	for x := 0; x < width; x++ {
		seen := false
		for y := 0; y < height; y++ {
			if b[y][x] != 0 {
				if !seen {
					hs[x] = height - y
					seen = true
				}
			} else if seen {
				f.Holes++
			}
		}
		f.HeightSum += hs[x]
		f.MaxHeight = max(f.MaxHeight, hs[x])
		if x > 0 {
			f.Bumpiness += int(math.Abs(float64(hs[x] - hs[x-1])))
		}
	}
	return f
}
func (g *game) moves() []move {
	moves := []move{}
	s := shapes[g.Piece]
	seen := map[string]bool{}
	old := measure(g.Board)
	for rot := 0; rot < 4; rot++ {
		// A cell set, independent of ordering, identifies equivalent rotations.
		key := ""
		for y := 0; y < 4; y++ {
			for x := 0; x < 4; x++ {
				for _, c := range s {
					if c.X == x && c.Y == y {
						key += fmt.Sprintf("%d%d", x, y)
					}
				}
			}
		}
		if seen[key] {
			s = rotate(s)
			continue
		}
		seen[key] = true
		for x := 0; x < width; x++ {
			if !fits(g.Board, s, x, 0) {
				continue
			}
			y := 0
			for fits(g.Board, s, x, y+1) {
				y++
			}
			m := move{ID: fmt.Sprintf("r%dx%d", rot, x), Rotation: rot, Column: x}
			b := g.Board
			for i, c := range s {
				m.Cells[i] = cell{x + c.X, y + c.Y}
				b[y+c.Y][x+c.X] = 1
			}
			after, lines := clearRows(b)
			m.Features = measure(after)
			m.Features.Lines = lines
			m.Features.AddedHoles = m.Features.Holes - old.Holes
			moves = append(moves, m)
		}
		s = rotate(s)
	}
	return moves
}
func (g *game) place(m move) {
	color := uint8(1)
	for i, n := range pieceNames {
		if n == g.Piece {
			color = uint8(i + 1)
		}
	}
	for _, c := range m.Cells {
		g.Board[c.Y][c.X] = color
	}
	var n int
	g.Board, n = clearRows(g.Board)
	level := g.Lines / 10
	g.Lines += n
	g.Score += []int{0, 100, 300, 500, 800}[n] * (level + 1)
	g.Turn++
	g.advance()
}
func heuristic(moves []move) move {
	best := moves[0]
	score := func(m move) float64 {
		f := m.Features
		return .760666*float64(f.Lines) - .510066*float64(f.HeightSum) - .35663*float64(f.Holes) - .184483*float64(f.Bumpiness)
	}
	for _, m := range moves[1:] {
		if score(m) > score(best) {
			best = m
		}
	}
	return best
}
