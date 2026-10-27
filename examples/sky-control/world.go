package main

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"
)

const holdX, holdY, holdRadius, holdSpeed = -12.0, 13.0, 6.0, 210.0
const freshnessSeconds = 45.0 // A simulation decision SLO, not an aviation rule.

type point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"alt_ft"`
}
type waypoint struct {
	To    point   `json:"to"`
	Speed float64 `json:"speed_kts"`
	Phase string  `json:"phase"`
}
type flight struct {
	ID        string     `json:"id"`
	Type      string     `json:"type"`
	Phase     string     `json:"phase"`
	Position  point      `json:"position"`
	Route     []waypoint `json:"route,omitempty"`
	Fuel      float64    `json:"fuel_seconds"`
	Wait      float64    `json:"wait_seconds"`
	Slot      int        `json:"slot"`
	Runway    string     `json:"runway,omitempty"`
	Touchdown float64    `json:"touchdown_at,omitempty"`
	Note      string     `json:"note,omitempty"`
	Finished  float64    `json:"finished_at,omitempty"`
}
type window struct {
	Flight string  `json:"flight"`
	Start  float64 `json:"start"`
	End    float64 `json:"end"`
}
type runway struct {
	ID     string   `json:"id"`
	Y      float64  `json:"y"`
	Closed bool     `json:"closed"`
	Slots  []window `json:"slots"`
}
type settings struct {
	Aircraft int     `json:"aircraft"`
	Arrivals float64 `json:"arrivals_per_minute"`
	Speed    float64 `json:"speed"`
}

func (c settings) valid() bool {
	return c.Aircraft >= 4 && c.Aircraft <= 24 && c.Arrivals >= 0 && c.Arrivals <= 6 && c.Speed >= 1 && c.Speed <= 60
}

type statistics struct {
	Landed      int `json:"landed"`
	Departed    int `json:"departed"`
	Crashes     int `json:"crashes"`
	SafetyStops int `json:"safety_stops"`
	Accepted    int `json:"accepted"`
	Unsafe      int `json:"unsafe"`
	Stale       int `json:"stale"`
	Late        int `json:"late"`
	Errors      int `json:"errors"`
	Decisions   int `json:"decisions"`
}
type world struct {
	Seed         int64      `json:"seed"`
	Seconds      float64    `json:"seconds"`
	Config       settings   `json:"config"`
	Flights      []flight   `json:"flights"`
	Runways      []runway   `json:"runways"`
	Stats        statistics `json:"stats"`
	Backlog      int        `json:"backlog"`
	Stopped      string     `json:"stopped,omitempty"`
	rng          *rand.Rand
	serial       int
	arrivalClock float64
}

func newWorld(seed int64, c settings) *world {
	w := &world{Seed: seed, Config: c, rng: rand.New(rand.NewSource(seed)), Runways: []runway{{ID: "09L", Y: 1.8}, {ID: "09R", Y: -1.8}}}
	w.fill()
	for i, r := range w.Runways {
		w.serial++
		w.Flights = append(w.Flights, flight{ID: fmt.Sprintf("DEP%03d", w.serial), Type: "MEDIUM", Phase: "ready", Position: point{-1, r.Y + math.Copysign(.65, r.Y), 0}, Slot: -1 - i, Fuel: 7200, Note: "Ready for departure; holding short of runway."})
	}
	return w
}
func (w *world) clone() *world {
	v := *w
	v.Flights = append([]flight(nil), w.Flights...)
	v.Runways = append([]runway(nil), w.Runways...)
	for i := range v.Runways {
		v.Runways[i].Slots = append([]window{}, w.Runways[i].Slots...)
	}
	v.rng = nil
	return &v
}
func (w *world) active() int {
	n := 0
	for _, p := range w.Flights {
		if p.airborne() {
			n++
		}
	}
	return n
}
func (p flight) airborne() bool {
	return p.Phase == "holding" || p.Phase == "approach" || p.Phase == "departing"
}
func (w *world) spawn() bool {
	used := map[int]bool{}
	for _, p := range w.Flights {
		if p.airborne() && p.Slot >= 0 {
			used[p.Slot] = true
		}
	}
	slot := -1
	for i := 0; i < 24; i++ {
		if !used[i] {
			slot = i
			break
		}
	}
	if slot < 0 {
		return false
	}
	w.serial++
	p := flight{ID: fmt.Sprintf("%s%03d", []string{"RHT", "SKY", "BAW", "AFR"}[w.rng.Intn(4)], w.serial), Type: "MEDIUM", Phase: "holding", Slot: slot, Fuel: float64(25+w.rng.Intn(25)) * 60, Note: "Routine arrival; request landing clearance."}
	if w.serial%5 == 0 {
		p.Type = "HEAVY"
		p.Note = "Heavy aircraft; allow the longer runway occupancy and wake buffer."
	}
	p.Position = holdingPosition(slot, w.Seconds)
	w.Flights = append(w.Flights, p)
	return true
}
func (w *world) fill() {
	for w.active() < w.Config.Aircraft {
		if !w.spawn() {
			break
		}
	}
}
func holdingPosition(slot int, clock float64) point {
	a := float64(slot%8)*math.Pi/4 + float64(slot/8)*math.Pi/12 + clock*(holdSpeed/3600)/holdRadius
	return point{holdX + holdRadius*math.Cos(a), holdY + holdRadius*math.Sin(a), 4000 + float64(slot/8)*1200}
}
func distance(a, b point) float64 { return math.Hypot(a.X-b.X, a.Y-b.Y) }
func legSeconds(from point, leg waypoint) float64 {
	return math.Max(distance(from, leg.To)/leg.Speed*3600, math.Abs(from.Z-leg.To.Z)/1800*60)
}
func advanceFlight(p flight, dt, clock float64) flight {
	if p.Phase == "holding" {
		p.Position = holdingPosition(p.Slot, clock+dt)
		p.Wait += dt
		return p
	}
	for dt > 1e-8 && len(p.Route) > 0 {
		leg := p.Route[0]
		duration := legSeconds(p.Position, leg)
		if duration <= dt+1e-8 {
			p.Position = leg.To
			dt -= duration
			p.Route = p.Route[1:]
			if len(p.Route) > 0 {
				p.Phase = p.Route[0].Phase
			} else if leg.Phase == "departing" {
				p.Phase = "exited"
			} else {
				p.Phase = "parked"
			}
			continue
		}
		f := dt / duration
		p.Position = point{p.Position.X + (leg.To.X-p.Position.X)*f, p.Position.Y + (leg.To.Y-p.Position.Y)*f, p.Position.Z + (leg.To.Z-p.Position.Z)*f}
		p.Phase = leg.Phase
		dt = 0
	}
	return p
}
func (w *world) advance(dt float64) {
	for dt > 1e-8 && w.Stopped == "" {
		step := math.Min(dt, .5)
		w.tick(step)
		dt -= step
	}
}
func (w *world) tick(dt float64) {
	next := make([]flight, len(w.Flights))
	for i, p := range w.Flights {
		next[i] = advanceFlight(p, dt, w.Seconds)
		if p.airborne() {
			next[i].Fuel -= dt
			if next[i].Fuel <= 120 {
				w.Stopped = "Fuel-reserve protection: " + p.ID + " reached the game's two-minute reserve."
				w.Stats.SafetyStops++
				return
			}
		}
	}
	for i := range next {
		for j := 0; j < i; j++ {
			if trajectoriesConflict(w.Flights[i], w.Flights[j], next[i], next[j], 0) {
				w.Stopped = "Separation protection: predicted conflict between " + next[i].ID + " and " + next[j].ID + ". Sector paused before movement."
				w.Stats.SafetyStops++
				return
			}
		}
	}
	w.Seconds += dt
	for i, p := range next {
		old := w.Flights[i]
		if old.Phase == "approach" && p.Phase != "approach" {
			w.Stats.Landed++
		}
		if old.Phase != "exited" && p.Phase == "exited" {
			w.Stats.Departed++
		}
		if (p.Phase == "parked" || p.Phase == "exited") && p.Finished == 0 {
			p.Finished = w.Seconds
		}
		next[i] = p
	}
	w.Flights = w.Flights[:0]
	for _, p := range next {
		if p.Finished == 0 || w.Seconds-p.Finished < 120 {
			w.Flights = append(w.Flights, p)
		}
	}
	for i := range w.Runways {
		slots := w.Runways[i].Slots[:0]
		for _, s := range w.Runways[i].Slots {
			if s.End > w.Seconds {
				slots = append(slots, s)
			}
		}
		w.Runways[i].Slots = slots
	}
	w.arrivalClock += dt * w.Config.Arrivals / 60
	for w.arrivalClock >= 1 {
		w.arrivalClock--
		if w.active() < w.Config.Aircraft && w.spawn() {
			if w.Backlog > 0 {
				w.Backlog--
			}
		} else {
			w.Backlog++
		}
	}
}

// Intersect continuous horizontal/vertical separation intervals over one step.
// This catches aircraft crossing between endpoints, rather than just testing
// distances at rendered frames. Terminal/surface margins are game simplifications.
func trajectoriesConflict(a, b, an, bn flight, buffer float64) bool {
	if a.Phase == "exited" || b.Phase == "exited" {
		return false
	}
	h, v := .08, 100.0
	if a.airborne() || b.airborne() {
		h, v = .3, 300
		if math.Min(math.Min(a.Position.Z, an.Position.Z), math.Min(b.Position.Z, bn.Position.Z)) >= 1000 {
			h, v = 3, 1000
		}
	}
	h += buffer
	x, y, z := a.Position.X-b.Position.X, a.Position.Y-b.Position.Y, a.Position.Z-b.Position.Z
	dx, dy, dz := an.Position.X-bn.Position.X-x, an.Position.Y-bn.Position.Y-y, an.Position.Z-bn.Position.Z-z
	lo, hi := 0.0, 1.0
	if math.Abs(dz) < 1e-10 {
		if math.Abs(z) >= v {
			return false
		}
	} else {
		v0, v1 := (-v-z)/dz, (v-z)/dz
		if v0 > v1 {
			v0, v1 = v1, v0
		}
		lo = math.Max(lo, v0)
		hi = math.Min(hi, v1)
		if lo >= hi {
			return false
		}
	}
	qa, qb, qc := dx*dx+dy*dy, 2*(x*dx+y*dy), x*x+y*y-h*h
	if qa < 1e-14 {
		return qc < 0 && lo < hi
	}
	disc := qb*qb - 4*qa*qc
	if disc <= 0 {
		return false
	}
	root := math.Sqrt(disc)
	lo = math.Max(lo, (-qb-root)/(2*qa))
	hi = math.Min(hi, (-qb+root)/(2*qa))
	return lo < hi-1e-10
}

type clearance struct {
	ID     string  `json:"id"`
	Flight string  `json:"flight,omitempty"`
	Kind   string  `json:"kind"`
	Runway string  `json:"runway,omitempty"`
	Safe   bool    `json:"safe_now"`
	Reason string  `json:"reason"`
	ETA    float64 `json:"eta_seconds,omitempty"`
}

func (w *world) plan(a clearance) (flight, window, error) {
	var p flight
	found := false
	for _, q := range w.Flights {
		if q.ID == a.Flight {
			p = q
			found = true
			break
		}
	}
	if !found {
		return p, window{}, errors.New("aircraft has left the sector")
	}
	var r runway
	for _, q := range w.Runways {
		if q.ID == a.Runway {
			r = q
			break
		}
	}
	if r.ID == "" {
		return p, window{}, errors.New("unknown runway")
	}
	if r.Closed {
		return p, window{}, errors.New("runway closed to new clearances")
	}
	var route []waypoint
	var slot window
	if a.Kind == "land" {
		if p.Phase != "holding" {
			return p, slot, errors.New("aircraft is no longer in the holding stack")
		}
		alt := p.Position.Z
		angle := math.Atan2(p.Position.Y-holdY, p.Position.X-holdX)
		route = append(route, waypoint{point{holdX + 10*math.Cos(angle), holdY + 10*math.Sin(angle), alt}, 210, "approach"})
		delta := math.Remainder(math.Pi-angle, 2*math.Pi)
		steps := int(math.Ceil(math.Abs(delta) / (.35)))
		for i := 1; i <= steps; i++ {
			a := angle + delta*float64(i)/float64(steps)
			route = append(route, waypoint{point{holdX + 10*math.Cos(a), holdY + 10*math.Sin(a), alt}, 210, "approach"})
		}
		route = append(route, waypoint{point{-24, r.Y, alt}, 210, "approach"}, waypoint{point{-1, r.Y, 0}, 180, "approach"})
		eta := 0.0
		from := p.Position
		for _, l := range route {
			eta += legSeconds(from, l)
			from = l.To
		}
		p.Touchdown = w.Seconds + eta
		buffer := 70.0
		if p.Type == "HEAVY" {
			buffer = 150
		}
		slot = window{p.ID, p.Touchdown, p.Touchdown + 2.0/85*3600 + buffer}
		sign := math.Copysign(1, r.Y)
		parking := 5 + float64(p.Slot%8)*.35
		route = append(route, waypoint{point{1, r.Y, 0}, 85, "rollout"}, waypoint{point{2.5, r.Y + sign*.9, 0}, 25, "taxi"}, waypoint{point{parking, r.Y + sign*(1.3+float64(p.Slot/8)*.4), 0}, 25, "taxi"})
	} else if a.Kind == "depart" {
		if p.Phase != "ready" {
			return p, slot, errors.New("aircraft is no longer holding short")
		}
		route = []waypoint{{point{-1, r.Y, 0}, 30, "taxi_out"}, {point{1, r.Y, 0}, 160, "takeoff"}, {point{20, r.Y, 6000}, 220, "departing"}}
		eta := legSeconds(p.Position, route[0]) + legSeconds(route[0].To, route[1])
		slot = window{p.ID, w.Seconds, w.Seconds + eta + 90}
	} else {
		return p, slot, errors.New("unknown clearance")
	}
	for _, s := range r.Slots {
		if slot.Start < s.End && s.Start < slot.End {
			return p, slot, fmt.Errorf("runway slot conflicts with %s", s.Flight)
		}
	}
	p.Route = route
	p.Phase = route[0].Phase
	p.Runway = r.ID
	return p, slot, nil
}
func (w *world) forecast(p flight) error {
	for _, q := range w.Flights {
		if q.ID == p.ID || q.Phase == "parked" || q.Phase == "exited" {
			continue
		}
		a, b := p, q
		for t := 0.0; t < 1200; t += 5 {
			an := advanceFlight(a, 5, w.Seconds+t)
			bn := advanceFlight(b, 5, w.Seconds+t)
			if trajectoriesConflict(a, b, an, bn, .12) {
				return fmt.Errorf("predicted separation conflict with %s", q.ID)
			}
			a, b = an, bn
		}
	}
	return nil
}
func (w *world) candidates() []clearance {
	out := []clearance{{ID: "wait", Kind: "wait", Safe: true, Reason: "Continue existing cleared routes and assigned holding patterns."}}
	for _, p := range w.Flights {
		kind := ""
		if p.Phase == "holding" {
			kind = "land"
		} else if p.Phase == "ready" {
			kind = "depart"
		}
		if kind == "" {
			continue
		}
		for _, r := range w.Runways {
			a := clearance{ID: kind + "_" + p.ID + "_" + r.ID, Flight: p.ID, Kind: kind, Runway: r.ID, Safe: true, Reason: "Runway reservation and forecast separation checks pass."}
			plan, reservation, err := w.plan(a)
			if err == nil {
				err = w.forecast(plan)
				if kind == "land" {
					a.ETA = math.Round(plan.Touchdown - w.Seconds)
				} else {
					a.ETA = math.Round(reservation.End - w.Seconds)
				}
			}
			if err != nil {
				a.Safe = false
				a.Reason = err.Error()
			}
			out = append(out, a)
		}
	}
	return out
}
func (w *world) apply(a clearance) error {
	if a.Kind == "wait" {
		return nil
	}
	p, slot, err := w.plan(a)
	if err != nil {
		return err
	}
	if err = w.forecast(p); err != nil {
		return err
	}
	for i := range w.Flights {
		if w.Flights[i].ID == p.ID {
			w.Flights[i] = p
			break
		}
	}
	for i := range w.Runways {
		if w.Runways[i].ID == a.Runway {
			w.Runways[i].Slots = append(w.Runways[i].Slots, slot)
			break
		}
	}
	return nil
}
func (w *world) reference(options []clearance) clearance {
	best := options[0]
	rank := -math.MaxFloat64
	for _, a := range options {
		if !a.Safe || a.Kind == "wait" {
			continue
		}
		for _, p := range w.Flights {
			if p.ID != a.Flight {
				continue
			}
			score := p.Wait / 60
			if a.Kind == "land" {
				score += 10000 - p.Fuel/60
				if p.Note != "Routine arrival; request landing clearance." && p.Type != "HEAVY" {
					score += 10000
				}
			}
			if score > rank {
				rank = score
				best = a
			}
		}
	}
	return best
}
func (w *world) emergency() string {
	ids := []int{}
	for i, p := range w.Flights {
		if p.Phase == "holding" {
			ids = append(ids, i)
		}
	}
	if len(ids) == 0 {
		return ""
	}
	sort.Slice(ids, func(i, j int) bool { return w.Flights[ids[i]].Fuel < w.Flights[ids[j]].Fuel })
	p := &w.Flights[ids[0]]
	p.Fuel = math.Min(p.Fuel, 18*60)
	p.Note = "MEDICAL EMERGENCY: unstable passenger; request priority landing."
	return p.ID
}
