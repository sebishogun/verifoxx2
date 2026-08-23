package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/sebishogun/verifoxx2/pkg/format"
	"github.com/sebishogun/verifoxx2/pkg/policy"
)

func main() {
	reqPath := flag.String("requests", "fixtures/requests.json", "Path to requests JSON file")
	evPath := flag.String("evidence", "fixtures/evidence.json", "Path to evidence JSON file")
	outPath := flag.String("output", "results/requests.json", "Path to write output results JSON")

	flag.Parse()

	requests, err := format.LoadRequests(*reqPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading requests: %v\n", err)
		os.Exit(1)
	}

	evidenceMap, err := format.LoadEvidence(*evPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading evidence: %v\n", err)
		os.Exit(1)
	}

	evaluator := policy.NewEvaluator()
	results := make([]policy.EvaluationResult, 0, len(requests))

	fmt.Println("Verifoxx Policy Engine - Evaluating Requests")
	fmt.Println("--------------------------------------------")

	for _, req := range requests {
		res := evaluator.Evaluate(req, evidenceMap)
		results = append(results, res)
		fmt.Printf("Request %-4s -> Decision: %-8s | Rationale: %s\n", res.RequestID, res.Decision, res.Rationale)
	}

	if err := format.WriteResults(*outPath, results); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing results: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nResults successfully saved to %s\n", *outPath)
}
