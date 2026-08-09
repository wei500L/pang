package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/dyhhhhhh/chatgpt-web-voice/internal/agentvisual"
)

func main() {
	input := flag.String("input", "assets/agent/source/dl.glb", "source GLB file")
	binaryOut := flag.String("binary", "static/models/agent-particles.bin", "particle binary output")
	manifestOut := flag.String("manifest", "static/models/agent-particles.json", "particle manifest output")
	count := flag.Int("particles", 65536, "number of uniformly sampled surface particles")
	seed := flag.Int64("seed", 3044, "deterministic sampling seed")
	anchors := flag.Int("anchors", 8, "number of propagation anchors")
	flag.Parse()

	manifest, err := agentvisual.Prepare(agentvisual.PrepareOptions{
		InputPath:     *input,
		BinaryPath:    *binaryOut,
		ManifestPath:  *manifestOut,
		ParticleCount: *count,
		Seed:          *seed,
		AnchorCount:   *anchors,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("prepared %d particles from %s\n", manifest.Count, manifest.Source.File)
	fmt.Printf("binary: %s\nmanifest: %s\n", *binaryOut, *manifestOut)
}
