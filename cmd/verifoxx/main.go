package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/sebishogun/verifoxx2/internal/adapters/jsonio"
	policycompile "github.com/sebishogun/verifoxx2/internal/compile"
	"github.com/sebishogun/verifoxx2/internal/eval"
	"github.com/sebishogun/verifoxx2/internal/result"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes the CLI with the given arguments and writers, returning the
// process exit code. Human progress and the decision table go to stderr;
// --output - emits only the OutputPack JSON on stdout.
func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("verifoxx", flag.ContinueOnError)
	flags.SetOutput(stderr)
	polPath := flags.String("policy", "policies/policy.json", "Path to policy JSON file")
	reqPath := flags.String("requests", "fixtures/requests.json", "Path to requests JSON file")
	evPath := flags.String("evidence", "fixtures/evidence.json", "Path to evidence JSON file")
	outPath := flags.String("output", "results/requests.json", "Path to write output results JSON")

	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "verifoxx: unexpected positional arguments: %v\n", flags.Args())
		return 2
	}

	policyAST, err := jsonio.LoadPolicy(*polPath)
	if err != nil {
		fmt.Fprintf(stderr, "Error loading policy AST: %v\n", err)
		return 1
	}
	compiled, diagnostics := policycompile.Compile(policyAST)
	if len(diagnostics) != 0 {
		for _, diagnostic := range diagnostics {
			fmt.Fprintf(stderr, "Error compiling policy at %s: %s\n", diagnostic.Path, diagnostic.Message)
		}
		return 1
	}

	requests, err := jsonio.LoadRequests(*reqPath)
	if err != nil {
		fmt.Fprintf(stderr, "Error loading requests: %v\n", err)
		return 1
	}

	evidence, err := jsonio.LoadEvidence(*evPath)
	if err != nil {
		fmt.Fprintf(stderr, "Error loading evidence: %v\n", err)
		return 1
	}

	var batch eval.Batch
	if err := eval.NewBuilder(&compiled, eval.DefaultLimits()).BuildInto(&batch, requests, evidence); err != nil {
		fmt.Fprintf(stderr, "Error building evaluation batch: %v\n", err)
		return 1
	}
	var context eval.Context
	context.Ensure(compiled, batch.Rows)
	var numeric result.Batch
	numeric.Ensure(batch.Rows, len(requests)*len(compiled.RequirementSymbols), len(batch.EvidenceRefs), len(requests)*len(compiled.Remediations))
	if err := eval.NewEvaluator(&compiled).EvaluateInto(&context, batch, &numeric); err != nil {
		fmt.Fprintf(stderr, "Error evaluating requests: %v\n", err)
		return 1
	}
	var pack jsonio.OutputPack
	if err := jsonio.MaterializeInto(&pack, compiled, batch, numeric, requests, evidence); err != nil {
		fmt.Fprintf(stderr, "Error materializing results: %v\n", err)
		return 1
	}

	fmt.Fprintf(stderr, "Verifoxx Policy Engine - Loaded Policy: %s v%s\n", compiled.Name, compiled.Version)
	fmt.Fprintln(stderr, "--------------------------------------------------------------------------------")
	for _, res := range pack.Results {
		fmt.Fprintf(stderr, "Request %-4s -> Decision: %-8s | Rationale: %s\n", res.RequestID, res.Decision, res.Rationale)
	}

	if *outPath == "-" {
		if err := jsonio.EncodeResults(stdout, pack); err != nil {
			fmt.Fprintf(stderr, "Error encoding results: %v\n", err)
			return 1
		}
		return 0
	}

	if err := jsonio.WriteResults(*outPath, pack); err != nil {
		fmt.Fprintf(stderr, "Error writing results: %v\n", err)
		return 1
	}

	fmt.Fprintf(stderr, "\nResults successfully saved to %s\n", *outPath)
	return 0
}
