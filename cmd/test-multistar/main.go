package main

import (
	"fmt"
)

// Test multi-star system distribution
func main() {
	total := 1000
	single := 0
	binary := 0
	trinary := 0
	
	// Star class counts for each position
	primaryClasses := make(map[string]int)
	secondaryClasses := make(map[string]int)
	tertiaryClasses := make(map[string]int)
	
	fmt.Printf("Generating %d star systems...\n\n", total)
	
	for i := 0; i < total; i++ {
		sys := NewSystem(fmt.Sprintf("System-%d", i), "localhost:8080", nil)
		
		if sys.Stars.IsTrinary {
			trinary++
		} else if sys.Stars.IsBinary {
			binary++
		} else {
			single++
		}
		
		// Track star classes
		primaryClasses[sys.Stars.Primary.Class]++
		if sys.Stars.Secondary != nil {
			secondaryClasses[sys.Stars.Secondary.Class]++
		}
		if sys.Stars.Tertiary != nil {
			tertiaryClasses[sys.Stars.Tertiary.Class]++
		}
	}
	
	fmt.Println("Multi-Star System Distribution:")
	fmt.Println("================================")
	fmt.Printf("Single-star systems:  %d (%.1f%%) [Expected: ~50%%]\n", single, float64(single)/float64(total)*100)
	fmt.Printf("Binary systems:       %d (%.1f%%) [Expected: ~40%%]\n", binary, float64(binary)/float64(total)*100)
	fmt.Printf("Trinary systems:      %d (%.1f%%) [Expected: ~10%%]\n", trinary, float64(trinary)/float64(total)*100)
	
	fmt.Println("\nPrimary Star Class Distribution:")
	fmt.Println("=================================")
	for _, class := range []string{"O", "B", "A", "F", "G", "K", "M"} {
		if count := primaryClasses[class]; count > 0 {
			fmt.Printf("%s Type: %d (%.2f%%)\n", class, count, float64(count)/float64(total)*100)
		}
	}
	
	if len(secondaryClasses) > 0 {
		fmt.Println("\nSecondary Star Class Distribution (Binary/Trinary only):")
		fmt.Println("=========================================================")
		for _, class := range []string{"O", "B", "A", "F", "G", "K", "M"} {
			if count := secondaryClasses[class]; count > 0 {
				fmt.Printf("%s Type: %d (%.2f%% of systems with secondary)\n", class, count, float64(count)/float64(binary+trinary)*100)
			}
		}
	}
	
	if len(tertiaryClasses) > 0 {
		fmt.Println("\nTertiary Star Class Distribution (Trinary only):")
		fmt.Println("================================================")
		for _, class := range []string{"O", "B", "A", "F", "G", "K", "M"} {
			if count := tertiaryClasses[class]; count > 0 {
				fmt.Printf("%s Type: %d (%.2f%% of trinary systems)\n", class, count, float64(count)/float64(trinary)*100)
			}
		}
	}
	
	fmt.Println("\nNote: Secondary/tertiary stars should skew toward smaller classes (M, K)")
	fmt.Println("as real binary/trinary systems typically have a large primary and smaller companions.")
}
