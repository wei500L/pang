# Agent visual source asset

`source/dl.glb` is the high-resolution source model for the voice-page particle body. It is kept for provenance and future regeneration, but it is not served to browsers or copied into the production container.

Regenerate the runtime particle asset after replacing the GLB:

```bash
go run ./cmd/prepare-agent-visual \
  -input assets/agent/source/dl.glb \
  -binary static/models/agent-particles.bin \
  -manifest static/models/agent-particles.json
```

The converter accepts standard triangle-based GLB files, including multiple meshes, node transforms, vertex colors, UVs, and embedded or relative base-color textures. Draco or Meshopt-compressed geometry must be decompressed first.
