package builtins

import "math"

// This is the public-domain SimplexNoise1234 implementation used by
// ToastStunt, translated directly to Go. The decimal skew constants are kept
// at Toast's source precision because those rounded values are observable.
var simplexPermutation = [...]int{
	151, 160, 137, 91, 90, 15, 131, 13, 201, 95, 96, 53, 194, 233, 7, 225,
	140, 36, 103, 30, 69, 142, 8, 99, 37, 240, 21, 10, 23, 190, 6, 148,
	247, 120, 234, 75, 0, 26, 197, 62, 94, 252, 219, 203, 117, 35, 11, 32,
	57, 177, 33, 88, 237, 149, 56, 87, 174, 20, 125, 136, 171, 168, 68, 175,
	74, 165, 71, 134, 139, 48, 27, 166, 77, 146, 158, 231, 83, 111, 229, 122,
	60, 211, 133, 230, 220, 105, 92, 41, 55, 46, 245, 40, 244, 102, 143, 54,
	65, 25, 63, 161, 1, 216, 80, 73, 209, 76, 132, 187, 208, 89, 18, 169,
	200, 196, 135, 130, 116, 188, 159, 86, 164, 100, 109, 198, 173, 186, 3, 64,
	52, 217, 226, 250, 124, 123, 5, 202, 38, 147, 118, 126, 255, 82, 85, 212,
	207, 206, 59, 227, 47, 16, 58, 17, 182, 189, 28, 42, 223, 183, 170, 213,
	119, 248, 152, 2, 44, 154, 163, 70, 221, 153, 101, 155, 167, 43, 172, 9,
	129, 22, 39, 253, 19, 98, 108, 110, 79, 113, 224, 232, 178, 185, 112, 104,
	218, 246, 97, 228, 251, 34, 242, 193, 238, 210, 144, 12, 191, 179, 162, 241,
	81, 51, 145, 235, 249, 14, 239, 107, 49, 192, 214, 31, 181, 199, 106, 157,
	184, 84, 204, 176, 115, 121, 50, 45, 127, 4, 150, 254, 138, 236, 205, 93,
	222, 114, 67, 29, 24, 72, 243, 141, 128, 195, 78, 66, 215, 61, 156, 180,
}

func simplexPerm(i int) int { return simplexPermutation[i&255] }

func simplexGrad1(hash int, x float64) float64 {
	h := hash & 15
	grad := float64(1 + (h & 7))
	if h&8 != 0 {
		grad = -grad
	}
	return grad * x
}

func simplexGrad2(hash int, x, y float64) float64 {
	h := hash & 7
	u, v := y, x
	if h < 4 {
		u, v = x, y
	}
	if h&1 != 0 {
		u = -u
	}
	if h&2 != 0 {
		v = -2 * v
	} else {
		v = 2 * v
	}
	return u + v
}

func simplexGrad3(hash int, x, y, z float64) float64 {
	h := hash & 15
	u := y
	if h < 8 {
		u = x
	}
	v := z
	if h < 4 {
		v = y
	} else if h == 12 || h == 14 {
		v = x
	}
	if h&1 != 0 {
		u = -u
	}
	if h&2 != 0 {
		v = -v
	}
	return u + v
}

func simplexGrad4(hash int, x, y, z, w float64) float64 {
	h := hash & 31
	u := y
	if h < 24 {
		u = x
	}
	v := z
	if h < 16 {
		v = y
	}
	q := w
	if h < 8 {
		q = z
	}
	if h&1 != 0 {
		u = -u
	}
	if h&2 != 0 {
		v = -v
	}
	if h&4 != 0 {
		q = -q
	}
	return u + v + q
}

func simplex1(x float64) float64 {
	i0 := int(math.Floor(x))
	x0, x1 := x-float64(i0), x-float64(i0)-1
	t0 := 1 - x0*x0
	t0 *= t0
	t1 := 1 - x1*x1
	t1 *= t1
	return 0.25 * (t0*t0*simplexGrad1(simplexPerm(i0), x0) + t1*t1*simplexGrad1(simplexPerm(i0+1), x1))
}

func simplex2(x, y float64) float64 {
	const f2, g2 = 0.366025403, 0.211324865
	s := (x + y) * f2
	i, j := int(math.Floor(x+s)), int(math.Floor(y+s))
	t := float64(i+j) * g2
	x0, y0 := x-(float64(i)-t), y-(float64(j)-t)
	i1, j1 := 0, 1
	if x0 > y0 {
		i1, j1 = 1, 0
	}
	x1, y1 := x0-float64(i1)+g2, y0-float64(j1)+g2
	x2, y2 := x0-1+2*g2, y0-1+2*g2
	ii, jj := i&255, j&255
	contribution := func(t, gx, gy float64, hash int) float64 {
		if t < 0 {
			return 0
		}
		t *= t
		return t * t * simplexGrad2(hash, gx, gy)
	}
	n0 := contribution(0.5-x0*x0-y0*y0, x0, y0, simplexPerm(ii+simplexPerm(jj)))
	n1 := contribution(0.5-x1*x1-y1*y1, x1, y1, simplexPerm(ii+i1+simplexPerm(jj+j1)))
	n2 := contribution(0.5-x2*x2-y2*y2, x2, y2, simplexPerm(ii+1+simplexPerm(jj+1)))
	return 40 * (n0 + n1 + n2)
}

func simplex3(x, y, z float64) float64 {
	const f3, g3 = 0.333333333, 0.166666667
	s := (x + y + z) * f3
	i, j, k := int(math.Floor(x+s)), int(math.Floor(y+s)), int(math.Floor(z+s))
	t := float64(i+j+k) * g3
	x0, y0, z0 := x-(float64(i)-t), y-(float64(j)-t), z-(float64(k)-t)
	i1, j1, k1, i2, j2, k2 := 0, 0, 0, 0, 0, 0
	if x0 >= y0 {
		if y0 >= z0 {
			i1, i2, j2 = 1, 1, 1
		} else if x0 >= z0 {
			i1, i2, k2 = 1, 1, 1
		} else {
			k1, i2, k2 = 1, 1, 1
		}
	} else {
		if y0 < z0 {
			k1, j2, k2 = 1, 1, 1
		} else if x0 < z0 {
			j1, j2, k2 = 1, 1, 1
		} else {
			j1, i2, j2 = 1, 1, 1
		}
	}
	x1, y1, z1 := x0-float64(i1)+g3, y0-float64(j1)+g3, z0-float64(k1)+g3
	x2, y2, z2 := x0-float64(i2)+2*g3, y0-float64(j2)+2*g3, z0-float64(k2)+2*g3
	x3, y3, z3 := x0-1+3*g3, y0-1+3*g3, z0-1+3*g3
	ii, jj, kk := i&255, j&255, k&255
	contribution := func(t, gx, gy, gz float64, hash int) float64 {
		if t < 0 {
			return 0
		}
		t *= t
		return t * t * simplexGrad3(hash, gx, gy, gz)
	}
	n0 := contribution(0.5-x0*x0-y0*y0-z0*z0, x0, y0, z0, simplexPerm(ii+simplexPerm(jj+simplexPerm(kk))))
	n1 := contribution(0.5-x1*x1-y1*y1-z1*z1, x1, y1, z1, simplexPerm(ii+i1+simplexPerm(jj+j1+simplexPerm(kk+k1))))
	n2 := contribution(0.5-x2*x2-y2*y2-z2*z2, x2, y2, z2, simplexPerm(ii+i2+simplexPerm(jj+j2+simplexPerm(kk+k2))))
	n3 := contribution(0.5-x3*x3-y3*y3-z3*z3, x3, y3, z3, simplexPerm(ii+1+simplexPerm(jj+1+simplexPerm(kk+1))))
	return 72 * (n0 + n1 + n2 + n3)
}

func simplex4(x, y, z, w float64) float64 {
	const f4, g4 = 0.309016994, 0.138196601
	s := (x + y + z + w) * f4
	i, j := int(math.Floor(x+s)), int(math.Floor(y+s))
	k, l := int(math.Floor(z+s)), int(math.Floor(w+s))
	t := float64(i+j+k+l) * g4
	x0, y0 := x-(float64(i)-t), y-(float64(j)-t)
	z0, w0 := z-(float64(k)-t), w-(float64(l)-t)
	rank := [4]int{}
	coords := [4]float64{x0, y0, z0, w0}
	for a := 0; a < 4; a++ {
		for b := a + 1; b < 4; b++ {
			if coords[a] > coords[b] {
				rank[a]++
			} else {
				rank[b]++
			}
		}
	}
	offset := func(threshold int) [4]int {
		var out [4]int
		for n := range out {
			if rank[n] >= threshold {
				out[n] = 1
			}
		}
		return out
	}
	o1, o2, o3 := offset(3), offset(2), offset(1)
	corner := func(o [4]int, scale float64) [4]float64 {
		return [4]float64{x0 - float64(o[0]) + scale*g4, y0 - float64(o[1]) + scale*g4, z0 - float64(o[2]) + scale*g4, w0 - float64(o[3]) + scale*g4}
	}
	c0 := [4]float64{x0, y0, z0, w0}
	c1, c2, c3 := corner(o1, 1), corner(o2, 2), corner(o3, 3)
	c4 := [4]float64{x0 - 1 + 4*g4, y0 - 1 + 4*g4, z0 - 1 + 4*g4, w0 - 1 + 4*g4}
	ii, jj, kk, ll := i&255, j&255, k&255, l&255
	hash := func(o [4]int) int {
		return simplexPerm(ii + o[0] + simplexPerm(jj+o[1]+simplexPerm(kk+o[2]+simplexPerm(ll+o[3]))))
	}
	contribution := func(c [4]float64, h int) float64 {
		t := 0.5 - c[0]*c[0] - c[1]*c[1] - c[2]*c[2] - c[3]*c[3]
		if t < 0 {
			return 0
		}
		t *= t
		return t * t * simplexGrad4(h, c[0], c[1], c[2], c[3])
	}
	ones := [4]int{1, 1, 1, 1}
	return 62 * (contribution(c0, hash([4]int{})) + contribution(c1, hash(o1)) + contribution(c2, hash(o2)) + contribution(c3, hash(o3)) + contribution(c4, hash(ones)))
}
