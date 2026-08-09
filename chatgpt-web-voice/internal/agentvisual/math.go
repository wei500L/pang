package agentvisual

import "math"

type vec2 struct {
	x, y float64
}

type vec3 struct {
	x, y, z float64
}

func add3(a, b vec3) vec3         { return vec3{a.x + b.x, a.y + b.y, a.z + b.z} }
func sub3(a, b vec3) vec3         { return vec3{a.x - b.x, a.y - b.y, a.z - b.z} }
func mul3(a vec3, s float64) vec3 { return vec3{a.x * s, a.y * s, a.z * s} }

func dot3(a, b vec3) float64 { return a.x*b.x + a.y*b.y + a.z*b.z }

func cross3(a, b vec3) vec3 {
	return vec3{
		a.y*b.z - a.z*b.y,
		a.z*b.x - a.x*b.z,
		a.x*b.y - a.y*b.x,
	}
}

func length3(v vec3) float64 { return math.Sqrt(dot3(v, v)) }

func normalize3(v vec3) vec3 {
	l := length3(v)
	if l < 1e-12 {
		return vec3{0, 0, 1}
	}
	return mul3(v, 1/l)
}

type mat4 [16]float64

func identity4() mat4 {
	return mat4{
		1, 0, 0, 0,
		0, 1, 0, 0,
		0, 0, 1, 0,
		0, 0, 0, 1,
	}
}

// mul4 multiplies column-major matrices and returns a*b.
func mul4(a, b mat4) mat4 {
	var out mat4
	for c := 0; c < 4; c++ {
		for r := 0; r < 4; r++ {
			out[c*4+r] =
				a[0*4+r]*b[c*4+0] +
					a[1*4+r]*b[c*4+1] +
					a[2*4+r]*b[c*4+2] +
					a[3*4+r]*b[c*4+3]
		}
	}
	return out
}

func transformPoint(m mat4, v vec3) vec3 {
	x := m[0]*v.x + m[4]*v.y + m[8]*v.z + m[12]
	y := m[1]*v.x + m[5]*v.y + m[9]*v.z + m[13]
	z := m[2]*v.x + m[6]*v.y + m[10]*v.z + m[14]
	w := m[3]*v.x + m[7]*v.y + m[11]*v.z + m[15]
	if math.Abs(w) > 1e-12 && math.Abs(w-1) > 1e-12 {
		return vec3{x / w, y / w, z / w}
	}
	return vec3{x, y, z}
}

type mat3 [9]float64

func normalMatrix(m mat4) mat3 {
	a00, a01, a02 := m[0], m[4], m[8]
	a10, a11, a12 := m[1], m[5], m[9]
	a20, a21, a22 := m[2], m[6], m[10]
	det := a00*(a11*a22-a12*a21) - a01*(a10*a22-a12*a20) + a02*(a10*a21-a11*a20)
	if math.Abs(det) < 1e-12 {
		return mat3{1, 0, 0, 0, 1, 0, 0, 0, 1}
	}
	id := 1 / det
	// Inverse-transpose, stored column-major.
	return mat3{
		(a11*a22 - a12*a21) * id,
		(a12*a20 - a10*a22) * id,
		(a10*a21 - a11*a20) * id,
		(a02*a21 - a01*a22) * id,
		(a00*a22 - a02*a20) * id,
		(a01*a20 - a00*a21) * id,
		(a01*a12 - a02*a11) * id,
		(a02*a10 - a00*a12) * id,
		(a00*a11 - a01*a10) * id,
	}
}

func transformNormal(m mat3, v vec3) vec3 {
	return normalize3(vec3{
		m[0]*v.x + m[3]*v.y + m[6]*v.z,
		m[1]*v.x + m[4]*v.y + m[7]*v.z,
		m[2]*v.x + m[5]*v.y + m[8]*v.z,
	})
}

func composeTRS(translation [3]float64, rotation [4]float64, scale [3]float64) mat4 {
	x, y, z, w := rotation[0], rotation[1], rotation[2], rotation[3]
	x2, y2, z2 := x+x, y+y, z+z
	xx, xy, xz := x*x2, x*y2, x*z2
	yy, yz, zz := y*y2, y*z2, z*z2
	wx, wy, wz := w*x2, w*y2, w*z2
	sx, sy, sz := scale[0], scale[1], scale[2]
	return mat4{
		(1 - (yy + zz)) * sx, (xy + wz) * sx, (xz - wy) * sx, 0,
		(xy - wz) * sy, (1 - (xx + zz)) * sy, (yz + wx) * sy, 0,
		(xz + wy) * sz, (yz - wx) * sz, (1 - (xx + yy)) * sz, 0,
		translation[0], translation[1], translation[2], 1,
	}
}

func octEncode(n vec3) vec2 {
	n = normalize3(n)
	denom := math.Abs(n.x) + math.Abs(n.y) + math.Abs(n.z)
	if denom < 1e-12 {
		return vec2{}
	}
	x, y := n.x/denom, n.y/denom
	if n.z < 0 {
		ox := (1 - math.Abs(y)) * signNotZero(x)
		oy := (1 - math.Abs(x)) * signNotZero(y)
		x, y = ox, oy
	}
	return vec2{x, y}
}

func signNotZero(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}
