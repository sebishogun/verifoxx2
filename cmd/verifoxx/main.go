package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/sebishogun/verifoxx2/internal/adapters/jsonio"
	"github.com/sebishogun/verifoxx2/internal/engine"
	"github.com/sebishogun/verifoxx2/internal/eval"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes the CLI with the given arguments and writers, returning the
// process exit code. Human progress and the decision table go to stderr.
// Stdout contains only one-shot OutputPack JSON or framed stream responses.
func run(args []string, stdout, stderr io.Writer) int {
	return runWithInput(args, os.Stdin, stdout, stderr)
}

func runWithInput(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("verifoxx", flag.ContinueOnError)
	flags.SetOutput(stderr)
	polPath := flags.String("policy", "policies/policy.json", "Path to policy JSON file")
	reqPath := flags.String("requests", "fixtures/requests.json", "Path to requests JSON file")
	evPath := flags.String("evidence", "fixtures/evidence.json", "Path to evidence JSON file")
	outPath := flags.String("output", "results/requests.json", "Path to write output results JSON")
	stream := flags.Bool("stream", false, "Read and write length-prefixed JSON frames")

	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "verifoxx: unexpected positional arguments: %v\n", flags.Args())
		return 2
	}
	if *stream {
		incompatible := ""
		flags.Visit(func(visited *flag.Flag) {
			switch visited.Name {
			case "requests", "evidence", "output":
				if incompatible == "" {
					incompatible = visited.Name
				}
			}
		})
		if incompatible != "" {
			fmt.Fprintf(stderr, "verifoxx: --%s cannot be used with --stream\n", incompatible)
			return 2
		}
	}

	policyAST, err := jsonio.LoadPolicy(*polPath)
	if err != nil {
		fmt.Fprintf(stderr, "Error loading policy AST: %v\n", err)
		return 1
	}
	runtime, diagnostics := engine.Compile(policyAST, eval.DefaultLimits())
	if len(diagnostics) != 0 {
		for _, diagnostic := range diagnostics {
			fmt.Fprintf(stderr, "Error compiling policy at %s: %s\n", diagnostic.Path, diagnostic.Message)
		}
		return 1
	}
	if *stream {
		return runStream(runtime, stdin, stdout, stderr)
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

	pack, err := runtime.NewSession().Evaluate(requests, evidence)
	if err != nil {
		fmt.Fprintf(stderr, "Error evaluating requests: %v\n", err)
		return 1
	}

	fmt.Fprintf(stderr, "Verifoxx Policy Engine - Loaded Policy: %s v%s\n", runtime.Name(), runtime.Version())
	fmt.Fprintln(stderr, "--------------------------------------------------------------------------------")
	for _, res := range pack.Results {
		fmt.Fprintf(stderr, "Request %-4s -> Decision: %-8s | Rationale: %s\n", res.RequestID, res.Decision, res.Rationale)
	}

	if *outPath == "-" {
		if err := jsonio.EncodeResults(stdout, *pack); err != nil {
			fmt.Fprintf(stderr, "Error encoding results: %v\n", err)
			return 1
		}
		return 0
	}

	if err := jsonio.WriteResults(*outPath, *pack); err != nil {
		fmt.Fprintf(stderr, "Error writing results: %v\n", err)
		return 1
	}

	fmt.Fprintf(stderr, "\nResults successfully saved to %s\n", *outPath)
	return 0
}
