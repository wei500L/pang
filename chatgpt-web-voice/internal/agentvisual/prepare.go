package agentvisual

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/qmuntal/gltf"
	"github.com/qmuntal/gltf/modeler"
)

const AssetVersion = 1

type PrepareOptions struct {
	InputPath     string
	BinaryPath    string
	ManifestPath  string
	ParticleCount int
	Seed          int64
	AnchorCount   int
}

type Manifest struct {
	Version       int                `json:"version"`
	Binary        string             `json:"binary"`
	Count         int                `json:"count"`
	Source        SourceInfo         `json:"source"`
	Normalization NormalizationInfo  `json:"normalization"`
	Sections      map[string]Section `json:"sections"`
	Anchors       [][3]float32       `json:"anchors"`
}

type SourceInfo struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type NormalizationInfo struct {
	Center           [3]float64    `json:"center"`
	Scale            float64       `json:"scale"`
	OriginalBounds   [2][3]float64 `json:"originalBounds"`
	NormalizedBounds [2][3]float64 `json:"normalizedBounds"`
}

type Section struct {
	Offset        int    `json:"offset"`
	ByteLength    int    `json:"byteLength"`
	ComponentType string `json:"componentType"`
	ItemSize      int    `json:"itemSize"`
	Normalized    bool   `json:"normalized"`
}

type rgba64 struct {
	r, g, b, a float64
}

type textureSource struct {
	img          image.Image
	wrapS, wrapT gltf.WrappingMode
}

type materialSource struct {
	factor  rgba64
	texture *textureSource
}

type primitiveSource struct {
	positions [][3]float32
	normals   [][3]float32
	uvs       [][2]float32
	colors    [][4]uint8
	indices   []uint32
	world     mat4
	normal    mat3
	material  materialSource
}

type triangleRef struct {
	primitive int
	base      int
}

type particle struct {
	position vec3
	normal   vec3
	color    rgba64
	seedA    float64
	seedB    float64
}

type bounds3 struct {
	min vec3
	max vec3
	set bool
}

func (b *bounds3) add(v vec3) {
	if !b.set {
		b.min, b.max, b.set = v, v, true
		return
	}
	b.min.x = math.Min(b.min.x, v.x)
	b.min.y = math.Min(b.min.y, v.y)
	b.min.z = math.Min(b.min.z, v.z)
	b.max.x = math.Max(b.max.x, v.x)
	b.max.y = math.Max(b.max.y, v.y)
	b.max.z = math.Max(b.max.z, v.z)
}

func Prepare(opts PrepareOptions) (*Manifest, error) {
	if opts.InputPath == "" || opts.BinaryPath == "" || opts.ManifestPath == "" {
		return nil, errors.New("input, binary output, and manifest output are required")
	}
	if opts.ParticleCount <= 0 {
		opts.ParticleCount = 65536
	}
	if opts.AnchorCount <= 0 {
		opts.AnchorCount = 8
	}
	if opts.Seed == 0 {
		opts.Seed = 3044
	}
	doc, err := gltf.Open(opts.InputPath)
	if err != nil {
		return nil, fmt.Errorf("open glb: %w", err)
	}
	if err := rejectCompressed(doc); err != nil {
		return nil, err
	}
	textures, err := loadTextures(doc, filepath.Dir(opts.InputPath))
	if err != nil {
		return nil, err
	}
	primitives, triangles, cumulative, bounds, err := collectGeometry(doc, textures)
	if err != nil {
		return nil, err
	}
	if len(triangles) == 0 || len(cumulative) == 0 || cumulative[len(cumulative)-1] <= 0 {
		return nil, errors.New("model contains no non-degenerate triangle surfaces")
	}

	center := mul3(add3(bounds.min, bounds.max), 0.5)
	extent := sub3(bounds.max, bounds.min)
	maxExtent := math.Max(extent.x, math.Max(extent.y, extent.z))
	if maxExtent <= 1e-12 {
		return nil, errors.New("model bounds have zero extent")
	}
	scale := 2 / maxExtent
	rng := rand.New(rand.NewSource(opts.Seed))
	particles := make([]particle, opts.ParticleCount)
	normalizedBounds := bounds3{}
	totalArea := cumulative[len(cumulative)-1]
	for i := range particles {
		pick := rng.Float64() * totalArea
		triIndex := sort.SearchFloat64s(cumulative, pick)
		if triIndex >= len(triangles) {
			triIndex = len(triangles) - 1
		}
		p := sampleTriangle(primitives[triangles[triIndex].primitive], triangles[triIndex].base, rng)
		p.position = mul3(sub3(p.position, center), scale)
		p.seedA = rng.Float64()
		p.seedB = rng.Float64()
		particles[i] = p
		normalizedBounds.add(p.position)
	}

	anchors := chooseAnchors(particles, opts.AnchorCount)
	sections, err := writeBinary(opts.BinaryPath, particles)
	if err != nil {
		return nil, err
	}
	source, err := sourceInfo(opts.InputPath)
	if err != nil {
		return nil, err
	}
	manifest := &Manifest{
		Version: AssetVersion,
		Binary:  filepath.Base(opts.BinaryPath),
		Count:   len(particles),
		Source:  source,
		Normalization: NormalizationInfo{
			Center:           array3(center),
			Scale:            scale,
			OriginalBounds:   [2][3]float64{array3(bounds.min), array3(bounds.max)},
			NormalizedBounds: [2][3]float64{array3(normalizedBounds.min), array3(normalizedBounds.max)},
		},
		Sections: sections,
		Anchors:  anchors,
	}
	if err := writeManifest(opts.ManifestPath, manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

func rejectCompressed(doc *gltf.Document) error {
	for _, name := range doc.ExtensionsRequired {
		if name == "KHR_draco_mesh_compression" || name == "EXT_meshopt_compression" {
			return fmt.Errorf("unsupported compressed GLB extension %s: decompress the model before preparing the particle asset", name)
		}
	}
	return nil
}

func collectGeometry(doc *gltf.Document, textures map[int]*textureSource) ([]primitiveSource, []triangleRef, []float64, bounds3, error) {
	var primitives []primitiveSource
	var triangles []triangleRef
	var cumulative []float64
	var bounds bounds3
	totalArea := 0.0
	sceneIndex := 0
	if doc.Scene != nil {
		sceneIndex = *doc.Scene
	}
	if sceneIndex < 0 || sceneIndex >= len(doc.Scenes) {
		return nil, nil, nil, bounds, fmt.Errorf("invalid scene index %d", sceneIndex)
	}
	var walk func(int, mat4) error
	walk = func(nodeIndex int, parent mat4) error {
		if nodeIndex < 0 || nodeIndex >= len(doc.Nodes) {
			return fmt.Errorf("invalid node index %d", nodeIndex)
		}
		node := doc.Nodes[nodeIndex]
		world := mul4(parent, nodeMatrix(node))
		if node.Mesh != nil {
			if *node.Mesh < 0 || *node.Mesh >= len(doc.Meshes) {
				return fmt.Errorf("node %q references invalid mesh %d", node.Name, *node.Mesh)
			}
			mesh := doc.Meshes[*node.Mesh]
			for primitiveIndex, primitive := range mesh.Primitives {
				if primitive.Mode != gltf.PrimitiveTriangles {
					continue
				}
				prepared, err := readPrimitive(doc, primitive, world, textures)
				if err != nil {
					return fmt.Errorf("mesh %q primitive %d: %w", mesh.Name, primitiveIndex, err)
				}
				pi := len(primitives)
				primitives = append(primitives, prepared)
				for i := 0; i+2 < len(prepared.indices); i += 3 {
					a, b, c := prepared.indices[i], prepared.indices[i+1], prepared.indices[i+2]
					if int(a) >= len(prepared.positions) || int(b) >= len(prepared.positions) || int(c) >= len(prepared.positions) {
						return fmt.Errorf("triangle index exceeds POSITION accessor")
					}
					pa := transformPoint(world, fromF32(prepared.positions[a]))
					pb := transformPoint(world, fromF32(prepared.positions[b]))
					pc := transformPoint(world, fromF32(prepared.positions[c]))
					area := length3(cross3(sub3(pb, pa), sub3(pc, pa))) * 0.5
					if area <= 1e-14 {
						continue
					}
					bounds.add(pa)
					bounds.add(pb)
					bounds.add(pc)
					totalArea += area
					triangles = append(triangles, triangleRef{primitive: pi, base: i})
					cumulative = append(cumulative, totalArea)
				}
			}
		}
		for _, child := range node.Children {
			if err := walk(child, world); err != nil {
				return err
			}
		}
		return nil
	}
	for _, root := range doc.Scenes[sceneIndex].Nodes {
		if err := walk(root, identity4()); err != nil {
			return nil, nil, nil, bounds, err
		}
	}
	return primitives, triangles, cumulative, bounds, nil
}

func readPrimitive(doc *gltf.Document, primitive *gltf.Primitive, world mat4, textures map[int]*textureSource) (primitiveSource, error) {
	positionAccessor, ok := primitive.Attributes[gltf.POSITION]
	if !ok || positionAccessor < 0 || positionAccessor >= len(doc.Accessors) {
		return primitiveSource{}, errors.New("missing POSITION accessor")
	}
	positions, err := modeler.ReadPosition(doc, doc.Accessors[positionAccessor], nil)
	if err != nil {
		return primitiveSource{}, fmt.Errorf("read positions: %w", err)
	}
	var indices []uint32
	if primitive.Indices != nil {
		indices, err = modeler.ReadIndices(doc, doc.Accessors[*primitive.Indices], nil)
		if err != nil {
			return primitiveSource{}, fmt.Errorf("read indices: %w", err)
		}
	} else {
		indices = make([]uint32, len(positions))
		for i := range indices {
			indices[i] = uint32(i)
		}
	}
	if len(indices)%3 != 0 {
		return primitiveSource{}, fmt.Errorf("triangle index count %d is not divisible by 3", len(indices))
	}
	var normals [][3]float32
	if idx, ok := primitive.Attributes[gltf.NORMAL]; ok {
		normals, err = modeler.ReadNormal(doc, doc.Accessors[idx], nil)
		if err != nil {
			return primitiveSource{}, fmt.Errorf("read normals: %w", err)
		}
	}
	var uvs [][2]float32
	if idx, ok := primitive.Attributes[gltf.TEXCOORD_0]; ok {
		uvs, err = modeler.ReadTextureCoord(doc, doc.Accessors[idx], nil)
		if err != nil {
			return primitiveSource{}, fmt.Errorf("read texture coordinates: %w", err)
		}
	}
	var colors [][4]uint8
	if idx, ok := primitive.Attributes[gltf.COLOR_0]; ok {
		colors, err = modeler.ReadColor(doc, doc.Accessors[idx], nil)
		if err != nil {
			return primitiveSource{}, fmt.Errorf("read colors: %w", err)
		}
	}
	return primitiveSource{
		positions: positions, normals: normals, uvs: uvs, colors: colors, indices: indices,
		world: world, normal: normalMatrix(world), material: materialForPrimitive(doc, primitive, textures),
	}, nil
}

func materialForPrimitive(doc *gltf.Document, primitive *gltf.Primitive, textures map[int]*textureSource) materialSource {
	result := materialSource{factor: rgba64{1, 1, 1, 1}}
	if primitive.Material == nil || *primitive.Material < 0 || *primitive.Material >= len(doc.Materials) {
		return result
	}
	material := doc.Materials[*primitive.Material]
	if material.PBRMetallicRoughness == nil {
		return result
	}
	pbr := material.PBRMetallicRoughness
	f := pbr.BaseColorFactorOrDefault()
	result.factor = rgba64{f[0], f[1], f[2], f[3]}
	if pbr.BaseColorTexture != nil && pbr.BaseColorTexture.TexCoord == 0 {
		result.texture = textures[pbr.BaseColorTexture.Index]
	}
	return result
}

func sampleTriangle(primitive primitiveSource, base int, rng *rand.Rand) particle {
	i0, i1, i2 := primitive.indices[base], primitive.indices[base+1], primitive.indices[base+2]
	r1, r2 := rng.Float64(), rng.Float64()
	sr1 := math.Sqrt(r1)
	w0, w1, w2 := 1-sr1, sr1*(1-r2), sr1*r2
	p0, p1, p2 := fromF32(primitive.positions[i0]), fromF32(primitive.positions[i1]), fromF32(primitive.positions[i2])
	local := add3(add3(mul3(p0, w0), mul3(p1, w1)), mul3(p2, w2))
	position := transformPoint(primitive.world, local)
	var normal vec3
	if len(primitive.normals) == len(primitive.positions) {
		n0, n1, n2 := fromF32(primitive.normals[i0]), fromF32(primitive.normals[i1]), fromF32(primitive.normals[i2])
		normal = transformNormal(primitive.normal, add3(add3(mul3(n0, w0), mul3(n1, w1)), mul3(n2, w2)))
	} else {
		wp0, wp1, wp2 := transformPoint(primitive.world, p0), transformPoint(primitive.world, p1), transformPoint(primitive.world, p2)
		normal = normalize3(cross3(sub3(wp1, wp0), sub3(wp2, wp0)))
	}
	vertexColor := rgba64{1, 1, 1, 1}
	if len(primitive.colors) == len(primitive.positions) {
		c0, c1, c2 := primitive.colors[i0], primitive.colors[i1], primitive.colors[i2]
		vertexColor = rgba64{
			(float64(c0[0])*w0 + float64(c1[0])*w1 + float64(c2[0])*w2) / 255,
			(float64(c0[1])*w0 + float64(c1[1])*w1 + float64(c2[1])*w2) / 255,
			(float64(c0[2])*w0 + float64(c1[2])*w1 + float64(c2[2])*w2) / 255,
			(float64(c0[3])*w0 + float64(c1[3])*w1 + float64(c2[3])*w2) / 255,
		}
	}
	texColor := rgba64{1, 1, 1, 1}
	if primitive.material.texture != nil && len(primitive.uvs) == len(primitive.positions) {
		u0, u1, u2 := primitive.uvs[i0], primitive.uvs[i1], primitive.uvs[i2]
		uv := vec2{
			float64(u0[0])*w0 + float64(u1[0])*w1 + float64(u2[0])*w2,
			float64(u0[1])*w0 + float64(u1[1])*w1 + float64(u2[1])*w2,
		}
		texColor = sampleTexture(*primitive.material.texture, uv)
	}
	return particle{
		position: position,
		normal:   normal,
		color:    multiplyColor(primitive.material.factor, multiplyColor(vertexColor, texColor)),
	}
}

func loadTextures(doc *gltf.Document, sourceDir string) (map[int]*textureSource, error) {
	required := make(map[int]struct{})
	for _, material := range doc.Materials {
		if material.PBRMetallicRoughness == nil || material.PBRMetallicRoughness.BaseColorTexture == nil {
			continue
		}
		required[material.PBRMetallicRoughness.BaseColorTexture.Index] = struct{}{}
	}
	decodedImages := make(map[int]image.Image)
	result := make(map[int]*textureSource)
	for textureIndex, texture := range doc.Textures {
		if _, ok := required[textureIndex]; !ok {
			continue
		}
		if texture.Source == nil || *texture.Source < 0 || *texture.Source >= len(doc.Images) {
			continue
		}
		imageIndex := *texture.Source
		img := decodedImages[imageIndex]
		if img == nil {
			data, err := imageBytes(doc, doc.Images[imageIndex], sourceDir)
			if err != nil {
				return nil, fmt.Errorf("texture %d: %w", textureIndex, err)
			}
			if len(data) == 0 {
				continue
			}
			img, _, err = image.Decode(bytes.NewReader(data))
			if err != nil {
				return nil, fmt.Errorf("decode texture %d: %w", textureIndex, err)
			}
			decodedImages[imageIndex] = img
		}
		wrapS, wrapT := gltf.WrapRepeat, gltf.WrapRepeat
		if texture.Sampler != nil && *texture.Sampler >= 0 && *texture.Sampler < len(doc.Samplers) {
			sampler := doc.Samplers[*texture.Sampler]
			wrapS, wrapT = sampler.WrapS, sampler.WrapT
		}
		result[textureIndex] = &textureSource{img: img, wrapS: wrapS, wrapT: wrapT}
	}
	return result, nil
}

func imageBytes(doc *gltf.Document, img *gltf.Image, sourceDir string) ([]byte, error) {
	if img.BufferView != nil {
		if *img.BufferView < 0 || *img.BufferView >= len(doc.BufferViews) {
			return nil, errors.New("invalid image bufferView")
		}
		return modeler.ReadBufferView(doc, doc.BufferViews[*img.BufferView])
	}
	if img.IsEmbeddedResource() {
		return img.MarshalData()
	}
	if img.URI == "" {
		return nil, nil
	}
	if strings.Contains(img.URI, "://") {
		return nil, errors.New("remote image URIs are not supported")
	}
	return os.ReadFile(filepath.Join(sourceDir, filepath.FromSlash(img.URI)))
}

func sampleTexture(texture textureSource, uv vec2) rgba64 {
	u := wrapCoordinate(uv.x, texture.wrapS)
	v := wrapCoordinate(uv.y, texture.wrapT)
	b := texture.img.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		return rgba64{1, 1, 1, 1}
	}
	x := u * float64(b.Dx()-1)
	y := (1 - v) * float64(b.Dy()-1)
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	x1, y1 := minInt(x0+1, b.Dx()-1), minInt(y0+1, b.Dy()-1)
	tx, ty := x-float64(x0), y-float64(y0)
	c00 := colorToRGBA(texture.img.At(b.Min.X+x0, b.Min.Y+y0))
	c10 := colorToRGBA(texture.img.At(b.Min.X+x1, b.Min.Y+y0))
	c01 := colorToRGBA(texture.img.At(b.Min.X+x0, b.Min.Y+y1))
	c11 := colorToRGBA(texture.img.At(b.Min.X+x1, b.Min.Y+y1))
	return mixColor(mixColor(c00, c10, tx), mixColor(c01, c11, tx), ty)
}

func wrapCoordinate(v float64, mode gltf.WrappingMode) float64 {
	switch mode {
	case gltf.WrapClampToEdge:
		return clamp01(v)
	case gltf.WrapMirroredRepeat:
		v = math.Mod(v, 2)
		if v < 0 {
			v += 2
		}
		if v > 1 {
			v = 2 - v
		}
		return v
	default:
		v = v - math.Floor(v)
		return v
	}
}

func colorToRGBA(c color.Color) rgba64 {
	r, g, b, a := c.RGBA()
	return rgba64{float64(r) / 65535, float64(g) / 65535, float64(b) / 65535, float64(a) / 65535}
}

func mixColor(a, b rgba64, t float64) rgba64 {
	return rgba64{a.r + (b.r-a.r)*t, a.g + (b.g-a.g)*t, a.b + (b.b-a.b)*t, a.a + (b.a-a.a)*t}
}

func multiplyColor(a, b rgba64) rgba64 { return rgba64{a.r * b.r, a.g * b.g, a.b * b.b, a.a * b.a} }

func writeBinary(path string, particles []particle) (map[string]Section, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create particle binary: %w", err)
	}
	defer f.Close()
	w := bufio.NewWriterSize(f, 1<<20)
	offset := 0
	sections := make(map[string]Section, 4)
	writeSection := func(name, component string, itemSize, componentBytes int, normalized bool, fn func(*bufio.Writer) error) error {
		length := len(particles) * itemSize * componentBytes
		sections[name] = Section{Offset: offset, ByteLength: length, ComponentType: component, ItemSize: itemSize, Normalized: normalized}
		if err := fn(w); err != nil {
			return err
		}
		offset += length
		return nil
	}
	if err := writeSection("position", "int16", 3, 2, true, func(w *bufio.Writer) error {
		for _, p := range particles {
			for _, v := range []float64{p.position.x, p.position.y, p.position.z} {
				if err := binary.Write(w, binary.LittleEndian, quantizeSNorm16(v)); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if err := writeSection("normal", "int16", 2, 2, true, func(w *bufio.Writer) error {
		for _, p := range particles {
			o := octEncode(p.normal)
			if err := binary.Write(w, binary.LittleEndian, quantizeSNorm16(o.x)); err != nil {
				return err
			}
			if err := binary.Write(w, binary.LittleEndian, quantizeSNorm16(o.y)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if err := writeSection("color", "uint8", 4, 1, true, func(w *bufio.Writer) error {
		for _, p := range particles {
			values := []byte{quantizeUNorm8(p.color.r), quantizeUNorm8(p.color.g), quantizeUNorm8(p.color.b), quantizeUNorm8(p.color.a)}
			if _, err := w.Write(values); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if err := writeSection("seed", "uint16", 2, 2, true, func(w *bufio.Writer) error {
		for _, p := range particles {
			if err := binary.Write(w, binary.LittleEndian, quantizeUNorm16(p.seedA)); err != nil {
				return err
			}
			if err := binary.Write(w, binary.LittleEndian, quantizeUNorm16(p.seedB)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if err := w.Flush(); err != nil {
		return nil, err
	}
	return sections, f.Sync()
}

func writeManifest(path string, manifest *Manifest) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func chooseAnchors(particles []particle, count int) [][3]float32 {
	if count > len(particles) {
		count = len(particles)
	}
	if count <= 0 {
		return nil
	}
	anchors := make([]vec3, 0, count)
	anchors = append(anchors, particles[0].position)
	for len(anchors) < count {
		bestDistance, bestIndex := -1.0, 0
		for i, p := range particles {
			nearest := math.MaxFloat64
			for _, a := range anchors {
				d := dot3(sub3(p.position, a), sub3(p.position, a))
				nearest = math.Min(nearest, d)
			}
			if nearest > bestDistance {
				bestDistance, bestIndex = nearest, i
			}
		}
		anchors = append(anchors, particles[bestIndex].position)
	}
	out := make([][3]float32, len(anchors))
	for i, a := range anchors {
		out[i] = [3]float32{float32(a.x), float32(a.y), float32(a.z)}
	}
	return out
}

func sourceInfo(path string) (SourceInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return SourceInfo{}, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return SourceInfo{}, err
	}
	return SourceInfo{File: filepath.Base(path), SHA256: hex.EncodeToString(h.Sum(nil)), Bytes: n}, nil
}

func nodeMatrix(node *gltf.Node) mat4 {
	translation := node.TranslationOrDefault()
	rotation := node.RotationOrDefault()
	scale := node.ScaleOrDefault()
	hasTRS := translation != [3]float64{} || rotation != [4]float64{0, 0, 0, 1} || scale != [3]float64{1, 1, 1}
	if hasTRS {
		return composeTRS(translation, rotation, scale)
	}
	return mat4(node.MatrixOrDefault())
}

func quantizeSNorm16(v float64) int16 {
	v = math.Max(-1, math.Min(1, v))
	return int16(math.Round(v * 32767))
}

func quantizeUNorm8(v float64) uint8   { return uint8(math.Round(clamp01(v) * 255)) }
func quantizeUNorm16(v float64) uint16 { return uint16(math.Round(clamp01(v) * 65535)) }
func clamp01(v float64) float64        { return math.Max(0, math.Min(1, v)) }
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func fromF32(v [3]float32) vec3 { return vec3{float64(v[0]), float64(v[1]), float64(v[2])} }
func array3(v vec3) [3]float64  { return [3]float64{v.x, v.y, v.z} }
