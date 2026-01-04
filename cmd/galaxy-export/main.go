package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"sync"
)

// GalaxySnapshot represents the state of the galaxy at a point in time
type GalaxySnapshot struct {
	Systems   []System `json:"systems"`
	Timestamp string   `json:"timestamp"`
	NodeCount int      `json:"node_count"`
}

func main() {
	var (
		nodes  = flag.String("nodes", "", "Comma-separated list of node addresses (e.g., localhost:8080,localhost:8081)")
		output = flag.String("output", "galaxy.json", "Output file path")
	)
	flag.Parse()

	if *nodes == "" {
		fmt.Println("Usage: galaxy-export -nodes <addr1,addr2,...> [-output galaxy.json]")
		os.Exit(1)
	}

	// Parse node addresses
	var nodeAddrs []string
	for _, addr := range splitCSV(*nodes) {
		nodeAddrs = append(nodeAddrs, addr)
	}

	fmt.Printf("Fetching data from %d nodes...\n", len(nodeAddrs))

	// Fetch system info from all nodes concurrently
	var wg sync.WaitGroup
	systemsChan := make(chan System, len(nodeAddrs))

	for _, addr := range nodeAddrs {
		wg.Add(1)
		go func(address string) {
			defer wg.Done()

			resp, err := http.Get("http://" + address + "/system")
			if err != nil {
				fmt.Printf("Failed to fetch from %s: %v\n", address, err)
				return
			}
			defer resp.Body.Close()

			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				fmt.Printf("Failed to read response from %s: %v\n", address, err)
				return
			}

			var sys System
			if err := json.Unmarshal(body, &sys); err != nil {
				fmt.Printf("Failed to parse response from %s: %v\n", address, err)
				return
			}

			systemsChan <- sys
			fmt.Printf("✓ Fetched: %s (%s %s)\n", sys.Name, sys.StarType.Class, sys.StarType.Description)
		}(addr)
	}

	// Wait for all fetches to complete
	go func() {
		wg.Wait()
		close(systemsChan)
	}()

	// Collect results
	var systems []System
	for sys := range systemsChan {
		systems = append(systems, sys)
	}

	if len(systems) == 0 {
		fmt.Println("No systems retrieved!")
		os.Exit(1)
	}

	// Create snapshot
	snapshot := GalaxySnapshot{
		Systems:   systems,
		Timestamp: systems[0].LastSeenAt.Format("2006-01-02T15:04:05Z"),
		NodeCount: len(systems),
	}

	// Write to file
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		fmt.Printf("Failed to marshal JSON: %v\n", err)
		os.Exit(1)
	}

	if err := ioutil.WriteFile(*output, data, 0644); err != nil {
		fmt.Printf("Failed to write file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n✓ Exported %d systems to %s\n", len(systems), *output)
	
	// Print summary statistics
	fmt.Println("\nGalaxy Statistics:")
	fmt.Println("==================")
	
	starCounts := make(map[string]int)
	for _, sys := range systems {
		starCounts[sys.StarType.Class]++
	}
	
	for _, class := range []string{"O", "B", "A", "F", "G", "K", "M"} {
		if count := starCounts[class]; count > 0 {
			fmt.Printf("%s Type: %d (%.1f%%)\n", class, count, float64(count)/float64(len(systems))*100)
		}
	}
	
	// Calculate average distances
	if len(systems) > 1 {
		var totalDist float64
		var pairs int
		for i := 0; i < len(systems); i++ {
			for j := i + 1; j < len(systems); j++ {
				totalDist += systems[i].DistanceTo(&systems[j])
				pairs++
			}
		}
		fmt.Printf("\nAverage inter-system distance: %.2f units\n", totalDist/float64(pairs))
	}
}

func splitCSV(s string) []string {
	var result []string
	current := ""
	for _, ch := range s {
		if ch == ',' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}
