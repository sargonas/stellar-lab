package main

import (
	"fmt"
	"math/rand"
	"time"
)

// Simple demo to show star type distribution
func main() {
	rand.Seed(time.Now().UnixNano())
	
	counts := map[string]int{
		"O": 0,
		"B": 0,
		"A": 0,
		"F": 0,
		"G": 0,
		"K": 0,
		"M": 0,
	}
	
	total := 10000
	fmt.Printf("Generating %d star systems...\n\n", total)
	
	for i := 0; i < total; i++ {
		sys := NewSystem(fmt.Sprintf("System-%d", i), "localhost:8080", nil)
		counts[sys.StarType.Class]++
	}
	
	fmt.Println("Star Type Distribution:")
	fmt.Println("=======================")
	fmt.Printf("O Type (Blue Supergiant): %d (%.3f%%)\n", counts["O"], float64(counts["O"])/float64(total)*100)
	fmt.Printf("B Type (Blue Giant):      %d (%.3f%%)\n", counts["B"], float64(counts["B"])/float64(total)*100)
	fmt.Printf("A Type (White Star):      %d (%.3f%%)\n", counts["A"], float64(counts["A"])/float64(total)*100)
	fmt.Printf("F Type (Yellow-White):    %d (%.3f%%)\n", counts["F"], float64(counts["F"])/float64(total)*100)
	fmt.Printf("G Type (Yellow Dwarf):    %d (%.3f%%)\n", counts["G"], float64(counts["G"])/float64(total)*100)
	fmt.Printf("K Type (Orange Dwarf):    %d (%.3f%%)\n", counts["K"], float64(counts["K"])/float64(total)*100)
	fmt.Printf("M Type (Red Dwarf):       %d (%.3f%%)\n", counts["M"], float64(counts["M"])/float64(total)*100)
	
	fmt.Println("\nExpected Distribution (Real Galaxy):")
	fmt.Println("=====================================")
	fmt.Println("O Type: ~0.003%")
	fmt.Println("B Type: ~0.13%")
	fmt.Println("A Type: ~0.6%")
	fmt.Println("F Type: ~3%")
	fmt.Println("G Type: ~8%")
	fmt.Println("K Type: ~12%")
	fmt.Println("M Type: ~76%")
}
