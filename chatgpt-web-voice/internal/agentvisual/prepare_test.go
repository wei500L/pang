package agentvisual

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/qmuntal/gltf"
	"github.com/qmuntal/gltf/modeler"
)

func TestPrepareDeterministicMultiNodeAsset(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	source := filepath.Join(dir, "fixture.glb")
	doc := gltf.NewDocument()
	doc.Meshes = []*gltf.Mesh{
		{Primitives: []*gltf.Primitive{{
			Attributes: gltf.PrimitiveAttributes{
				gltf.POSITION: modeler.WritePosition(doc, [][3]float32{{0, 0, 0}, {1, 0, 0}, {0, 1, 0}}),
				gltf.NORMAL:   modeler.WriteNormal(doc, [][3]float32{{0, 0, 1}, {0, 0, 1}, {0, 0, 1}}),
			},
			Indices: gltf.Index(modeler.WriteIndices(doc, []uint16{0, 1, 2})),
		}}},
		{Primitives: []*gltf.Primitive{{
			Attributes: gltf.PrimitiveAttributes{
				gltf.POSITION: modeler.WritePosition(doc, [][3]float32{{0, 0, 0}, {2, 0, 0}, {0, 2, 0}}),
			},
			Indices: gltf.Index(modeler.WriteIndices(doc, []uint16{0, 1, 2})),
		}}},
	}
	doc.Nodes = []*gltf.Node{
		{Mesh: gltf.Index(0)},
		{Mesh: gltf.Index(1), Translation: [3]float64{2, 0, 0}, Scale: [3]float64{1, 1, 1}, Rotation: [4]float64{0, 0, 0, 1}},
	}
	doc.Scenes[0].Nodes = []int{0, 1}
	if err := gltf.SaveBinary(doc, source); err != nil {
		t.Fatal(err)
	}

	prepare := func(name string) (*Manifest, []byte) {
		binaryPath := filepath.Join(dir, name+".bin")
		manifestPath := filepath.Join(dir, name+".json")
		manifest, err := Prepare(PrepareOptions{
			InputPath: source, BinaryPath: binaryPath, ManifestPath: manifestPath,
			ParticleCount: 1024, Seed: 77, AnchorCount: 4,
		})
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(binaryPath)
		if err != nil {
			t.Fatal(err)
		}
		return manifest, data
	}
	firstManifest, firstData := prepare("first")
	secondManifest, secondData := prepare("second")
	if !bytes.Equal(firstData, secondData) {
		t.Fatal("same seed produced different particle data")
	}
	if firstManifest.Count != 1024 || len(firstManifest.Anchors) != 4 {
		t.Fatalf("manifest count=%d anchors=%d", firstManifest.Count, len(firstManifest.Anchors))
	}
	if firstManifest.Source.SHA256 != secondManifest.Source.SHA256 {
		t.Fatal("source digest changed between runs")
	}
	if got, want := len(firstData), 1024*18; got != want {
		t.Fatalf("binary bytes=%d want=%d", got, want)
	}
	if firstManifest.Normalization.OriginalBounds[1][0] < 3.99 {
		t.Fatalf("node translation was not included in bounds: %#v", firstManifest.Normalization.OriginalBounds)
	}
	if !reflect.DeepEqual(firstManifest.Sections, secondManifest.Sections) {
		t.Fatal("section layout changed between deterministic runs")
	}
}

func TestSampleTextureWrapAndInterpolation(t *testing.T) {
	t.Parallel()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	img.Set(1, 0, color.NRGBA{G: 255, A: 255})
	img.Set(0, 1, color.NRGBA{B: 255, A: 255})
	img.Set(1, 1, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	tex := textureSource{img: img, wrapS: gltf.WrapClampToEdge, wrapT: gltf.WrapClampToEdge}
	c := sampleTexture(tex, vec2{0.5, 0.5})
	for name, value := range map[string]float64{"r": c.r, "g": c.g, "b": c.b, "a": c.a} {
		if value < 0.49 || value > 1.01 {
			t.Fatalf("%s interpolation=%f", name, value)
		}
	}
}

func TestImageDecodeFixture(t *testing.T) {
	t.Parallel()
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.NRGBA{R: 200, G: 100, B: 50, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	decoded, _, err := image.Decode(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	c := colorToRGBA(decoded.At(0, 0))
	if c.r < 0.75 || c.g < 0.35 || c.b < 0.15 || c.a != 1 {
		t.Fatalf("decoded color=%+v", c)
	}
}

func TestOctEncodeAndTRS(t *testing.T) {
	t.Parallel()
	n := normalize3(vec3{0.2, -0.4, 0.9})
	o := octEncode(n)
	if o.x < -1 || o.x > 1 || o.y < -1 || o.y > 1 {
		t.Fatalf("oct value out of range: %+v", o)
	}
	m := composeTRS([3]float64{3, 4, 5}, [4]float64{0, 0, 0, 1}, [3]float64{2, 2, 2})
	p := transformPoint(m, vec3{1, 0, 0})
	if p != (vec3{5, 4, 5}) {
		t.Fatalf("transformed point=%+v", p)
	}
}
