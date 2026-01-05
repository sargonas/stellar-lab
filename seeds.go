package main

import (
	"bufio"
	"log"
	"net/http"
	"strings"
	"time"
)

// SeedNodeListURL is the URL to fetch the seed node list from
const SeedNodeListURL = "https://raw.githubusercontent.com/sargonas/stellar-lab/main/SEED-NODES.txt"

// FallbackSeedNodes are used if GitHub is unreachable
var FallbackSeedNodes = []string{
	// Add stable fallback seeds here if needed
}

// FetchSeedNodes retrieves the current seed node list from GitHub
func FetchSeedNodes() []string {
	log.Printf("Fetching seed node list from GitHub...")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest("GET", SeedNodeListURL, nil)
	if err != nil {
		log.Printf("Warning: Could not create request: %v", err)
		log.Printf("Using fallback seed nodes")
		return FallbackSeedNodes
	}
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Warning: Could not fetch seed list from GitHub: %v", err)
		log.Printf("Using fallback seed nodes")
		return FallbackSeedNodes
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("Warning: GitHub seed list returned status %d", resp.StatusCode)
		log.Printf("Using fallback seed nodes")
		return FallbackSeedNodes
	}

	var seeds []string
	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		seeds = append(seeds, line)
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Warning: Error reading seed list: %v", err)
		log.Printf("Using fallback seed nodes")
		return FallbackSeedNodes
	}

	if len(seeds) == 0 {
		log.Printf("Warning: No seeds found in GitHub list")
		log.Printf("Using fallback seed nodes")
		return FallbackSeedNodes
	}

	log.Printf("Loaded %d seed nodes from GitHub", len(seeds))
	return seeds
}
