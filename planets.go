package main

import (
	"math"
)

// Planet represents a planet in the system
type Planet struct {
	Name         string  `json:"name"`
	Type         string  `json:"type"`          // "Rocky", "Gas Giant", "Ice Giant", "Lava"
	OrbitAU      float64 `json:"orbit_au"`      // Distance from star in AU
	MassEarths   float64 `json:"mass_earths"`   // Mass relative to Earth
	RadiusEarths float64 `json:"radius_earths"` // Radius relative to Earth
	IsHabitable  bool    `json:"is_habitable"`  // Within habitable zone?
}

// PlanetarySystem represents all planets in a star system
type PlanetarySystem struct {
	Planets        []Planet `json:"planets"`
	PlanetCount    int      `json:"planet_count"`
	HabitableCount int      `json:"habitable_count"`
	HasGasGiant    bool     `json:"has_gas_giant"`
}

// GeneratePlanetarySystem creates deterministic planets based on star system
func (s *System) GeneratePlanetarySystem() PlanetarySystem {
	// Planet count depends on primary star type
	var maxPlanets int
	var habitableZoneInner, habitableZoneOuter float64
	
	primaryClass := s.Stars.Primary.Class
	primaryLum := s.Stars.Primary.Luminosity
	
	// Determine max planets and habitable zone
	switch primaryClass {
	case "O", "B":
		maxPlanets = 3
		habitableZoneInner = math.Sqrt(primaryLum) * 30.0
		habitableZoneOuter = math.Sqrt(primaryLum) * 60.0
	case "A":
		maxPlanets = 5
		habitableZoneInner = math.Sqrt(primaryLum) * 8.0
		habitableZoneOuter = math.Sqrt(primaryLum) * 15.0
	case "F":
		maxPlanets = 7
		habitableZoneInner = math.Sqrt(primaryLum) * 1.5
		habitableZoneOuter = math.Sqrt(primaryLum) * 2.5
	case "G":
		maxPlanets = 8
		habitableZoneInner = math.Sqrt(primaryLum) * 0.95
		habitableZoneOuter = math.Sqrt(primaryLum) * 1.37
	case "K":
		maxPlanets = 9
		habitableZoneInner = math.Sqrt(primaryLum) * 0.3
		habitableZoneOuter = math.Sqrt(primaryLum) * 0.5
	case "M":
		maxPlanets = 10
		habitableZoneInner = math.Sqrt(primaryLum) * 0.02
		habitableZoneOuter = math.Sqrt(primaryLum) * 0.1
	default:
		maxPlanets = 5
		habitableZoneInner = 0.95
		habitableZoneOuter = 1.37
	}
	
	// Binary/trinary systems have fewer planets
	if s.Stars.IsBinary {
		maxPlanets = maxPlanets / 2
	}
	if s.Stars.IsTrinary {
		maxPlanets = maxPlanets / 3
	}
	if maxPlanets < 1 {
		maxPlanets = 1
	}
	
	planetCountSeed := s.DeterministicSeed("planet_count")
	planetCount := int(planetCountSeed%uint64(maxPlanets)) + 1
	
	planets := make([]Planet, planetCount)
	habitableCount := 0
	hasGasGiant := false
	
	for i := 0; i < planetCount; i++ {
		planet := s.generatePlanet(i, habitableZoneInner, habitableZoneOuter)
		planets[i] = planet
		
		if planet.IsHabitable {
			habitableCount++
		}
		if planet.Type == "Gas Giant" || planet.Type == "Ice Giant" {
			hasGasGiant = true
		}
	}
	
	return PlanetarySystem{
		Planets:        planets,
		PlanetCount:    planetCount,
		HabitableCount: habitableCount,
		HasGasGiant:    hasGasGiant,
	}
}

func (s *System) generatePlanet(index int, habitableInner, habitableOuter float64) Planet {
	seedSalt := "planet_" + string(rune(index))
	seed := s.DeterministicSeed(seedSalt)
	
	orbitBase := float64(index) * 0.3
	orbitVariation := float64(seed%1000) / 1000.0
	orbitAU := math.Pow(1.5, orbitBase) * (0.3 + orbitVariation)
	
	var planetType string
	var massEarths, radiusEarths float64
	
	if orbitAU < 0.5 {
		planetType = "Lava"
		massEarths = 0.1 + float64(seed%200)/100.0
		radiusEarths = 0.5 + float64(seed%150)/100.0
	} else if orbitAU < 2.0 {
		planetType = "Rocky"
		massEarths = 0.05 + float64(seed%300)/100.0
		radiusEarths = 0.4 + float64(seed%200)/100.0
	} else if orbitAU < 8.0 {
		if seed%10 < 7 {
			planetType = "Gas Giant"
			massEarths = 10.0 + float64(seed%30000)/100.0
			radiusEarths = 3.0 + float64(seed%1000)/100.0
		} else {
			planetType = "Rocky"
			massEarths = 0.5 + float64(seed%500)/100.0
			radiusEarths = 0.7 + float64(seed%250)/100.0
		}
	} else {
		planetType = "Ice Giant"
		massEarths = 5.0 + float64(seed%2000)/100.0
		radiusEarths = 2.0 + float64(seed%500)/100.0
	}
	
	isHabitable := false
	if planetType == "Rocky" && orbitAU >= habitableInner && orbitAU <= habitableOuter {
		if massEarths >= 0.3 && massEarths <= 10.0 {
			if radiusEarths >= 0.5 && radiusEarths <= 2.5 {
				isHabitable = true
			}
		}
	}
	
	name := s.generatePlanetName(index)
	
	return Planet{
		Name:         name,
		Type:         planetType,
		OrbitAU:      orbitAU,
		MassEarths:   massEarths,
		RadiusEarths: radiusEarths,
		IsHabitable:  isHabitable,
	}
}

func (s *System) generatePlanetName(index int) string {
	romanNumerals := []string{"I", "II", "III", "IV", "V", "VI", "VII", "VIII", "IX", "X"}
	if index < len(romanNumerals) {
		return s.Name + " " + romanNumerals[index]
	}
	return s.Name + " " + string(rune('A'+index))
}
