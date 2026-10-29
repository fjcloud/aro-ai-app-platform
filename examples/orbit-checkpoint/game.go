package main

import (
	"fmt"
	"math/rand"
)

const shiftLength = 12

var actions = []string{"clear", "inspect", "reject"}

const standingOrders = `You are the customs officer at EUROPA PORT. Today is 2037-04-17.
Apply rules in this order:
1. REJECT if the passport and permit names differ, the destination is not EUROPA, or the permit expires before today. A permit expiring today is valid.
2. REJECT any shipment of VOID CRYSTALS, regardless of licences or exemptions.
3. INSPECT medical supplies with a BROKEN seal.
4. Apply the current shift directive below.
5. Other cargo needs a cargo licence. If it is MISSING, INSPECT; if PRESENT, CLEAR.
The document image is the evidence. CLEAR means admit the courier; INSPECT means send to secondary inspection; REJECT means deny entry.`

type directive struct {
	Name string `json:"name"`
	Text string `json:"text"`
}

var directives = []directive{
	{"Medical fast lane", "SEALED MEDICAL SUPPLIES may be CLEARED without a cargo licence. All other standing rules still apply."},
	{"Relic quarantine", "All ANCIENT RELICS must be INSPECTED, even with a cargo licence. All other standing rules still apply."},
	{"Diplomatic corridor", "DIPLOMATIC couriers may be CLEARED without a cargo licence. All other standing rules still apply."},
}

// These fields stay server-side. The player and the model see the same rendered image.
type dossier struct {
	Name, PermitName, Origin, Destination, Expires string
	Cargo, Licence, Seal, Status, Reference        string
	Layout                                         int
}

type resolution struct {
	Round        int      `json:"round"`
	Expected     string   `json:"expected"`
	Explanation  string   `json:"explanation"`
	Human        string   `json:"human,omitempty"`
	HumanCorrect bool     `json:"human_correct"`
	ModelCorrect bool     `json:"model_correct"`
	Decision     decision `json:"decision"`
}

type shift struct {
	Seed                                                   int64
	Round                                                  int
	Cases                                                  []dossier
	Result                                                 *resolution
	History                                                []resolution
	HumanPlayed, HumanCorrect, ModelCorrect, Streak, Score int
}

func newShift(seed int64) *shift {
	rng := rand.New(rand.NewSource(seed))
	first := []string{"NOVA", "ARLO", "MIRA", "AXEL", "VERA", "LEO", "SORA", "ZURI"}
	last := []string{"VALE", "REED", "PARK", "SOL", "VOSS", "YU", "NASH", "KEEN"}
	origins := []string{"MARS", "TITAN", "LUNA", "CERES"}
	s := &shift{Seed: seed}
	// A shuffled deck samples valid, invalid and exceptional documents. The verdict
	// is calculated from the actual fields and directive, never from the deck index.
	for block := 0; len(s.Cases) < shiftLength; block++ {
		for _, kind := range rng.Perm(12) {
			d := dossier{Name: first[rng.Intn(len(first))] + " " + last[rng.Intn(len(last))],
				Origin: origins[rng.Intn(len(origins))], Destination: "EUROPA", Expires: "2037-06-30",
				Cargo: "MACHINE PARTS", Licence: "PRESENT", Seal: "INTACT", Status: "COMMERCIAL",
				Reference: fmt.Sprintf("EP-%04d", rng.Intn(9000)+1000), Layout: rng.Intn(2)}
			d.PermitName = d.Name
			switch kind {
			case 1:
				d.Expires = "2037-04-17" // Inclusive boundary.
			case 2:
				d.Expires = "2037-04-10"
			case 3:
				d.PermitName = "IVAN CROSS"
			case 4:
				d.Destination = "IO"
			case 5:
				d.Cargo = "VOID CRYSTALS"
			case 6:
				d.Licence = "MISSING"
			case 7:
				d.Cargo, d.Licence, d.Seal = "MEDICAL SUPPLIES", "MISSING", "SEALED"
			case 8:
				d.Cargo, d.Seal = "MEDICAL SUPPLIES", "BROKEN"
			case 9:
				d.Cargo = "ANCIENT RELICS"
			case 10:
				d.Status, d.Licence = "DIPLOMATIC", "MISSING"
			case 11:
				d.Cargo, d.Status = "VOID CRYSTALS", "DIPLOMATIC" // Rule priority.
			}
			s.Cases = append(s.Cases, d)
			if len(s.Cases) == shiftLength {
				break
			}
		}
	}
	return s
}

func (s *shift) directive() directive { return directives[(s.Round/4)%len(directives)] }
func (s *shift) over() bool           { return s.Round >= len(s.Cases) }

func verdict(d dossier, rule directive) (string, string) {
	switch {
	case d.Name != d.PermitName:
		return "reject", "Rule 1: the passport and permit names do not match."
	case d.Destination != "EUROPA":
		return "reject", "Rule 1: this permit is for a different destination."
	case d.Expires < "2037-04-17":
		return "reject", "Rule 1: the permit expired before today's arrival."
	case d.Cargo == "VOID CRYSTALS":
		return "reject", "Rule 2: Void Crystals are prohibited, including for diplomats."
	case d.Cargo == "MEDICAL SUPPLIES" && d.Seal == "BROKEN":
		return "inspect", "Rule 3: medical supplies with a broken seal need inspection."
	case rule.Name == "Medical fast lane" && d.Cargo == "MEDICAL SUPPLIES" && d.Seal == "SEALED":
		return "clear", "Shift directive: sealed medical supplies are exempt from the cargo licence."
	case rule.Name == "Relic quarantine" && d.Cargo == "ANCIENT RELICS":
		return "inspect", "Shift directive: ancient relics require inspection, even with a licence."
	case rule.Name == "Diplomatic corridor" && d.Status == "DIPLOMATIC":
		return "clear", "Shift directive: diplomatic couriers are exempt from the cargo licence."
	case d.Licence == "MISSING":
		return "inspect", "Rule 5: the cargo licence is missing and no exemption applies."
	default:
		return "clear", "The documents match, the permit is valid and the cargo licence is present."
	}
}

func (s *shift) resolve(human string, d decision) resolution {
	expected, why := verdict(s.Cases[s.Round], s.directive())
	r := resolution{Round: s.Round + 1, Expected: expected, Explanation: why, Decision: d,
		Human: human, HumanCorrect: human == expected, ModelCorrect: d.Choice == expected}
	if human != "" {
		s.HumanPlayed++
		if r.HumanCorrect {
			s.HumanCorrect++
			s.Streak++
			s.Score += 100 + 25*(s.Streak-1)
		} else {
			s.Streak = 0
		}
	}
	if r.ModelCorrect {
		s.ModelCorrect++
	}
	s.Result = &r
	s.History = append(s.History, r)
	return r
}
