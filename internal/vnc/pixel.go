package vnc

import (
	"encoding/binary"
	"fmt"
	"slices"
)

// pixelFormatLength is the size of an RFB PIXEL_FORMAT.
const pixelFormatLength = 16

// pixelFormat is an RFB PIXEL_FORMAT.
type pixelFormat struct {
	bitsPerPixel uint8
	depth        uint8
	bigEndian    bool
	trueColour   bool
	redMax       uint16
	greenMax     uint16
	blueMax      uint16
	redShift     uint8
	greenShift   uint8
	blueShift    uint8
}

// reducedFormat is the 16-bit colour (RGB 5-6-5) the app asks macOS Screen
// Sharing for. It halves what the server sends compared with 32-bit colour.
var reducedFormat = pixelFormat{
	bitsPerPixel: 16, depth: 16, trueColour: true,
	redMax: 31, greenMax: 63, blueMax: 31,
	redShift: 11, greenShift: 5, blueShift: 0,
}

func parsePixelFormat(b []byte) pixelFormat {
	return pixelFormat{
		bitsPerPixel: b[0],
		depth:        b[1],
		bigEndian:    b[2] != 0,
		trueColour:   b[3] != 0,
		redMax:       binary.BigEndian.Uint16(b[4:6]),
		greenMax:     binary.BigEndian.Uint16(b[6:8]),
		blueMax:      binary.BigEndian.Uint16(b[8:10]),
		redShift:     b[10],
		greenShift:   b[11],
		blueShift:    b[12],
	}
}

func (pf pixelFormat) bytes() []byte {
	b := make([]byte, pixelFormatLength)
	b[0], b[1] = pf.bitsPerPixel, pf.depth
	if pf.bigEndian {
		b[2] = 1
	}
	if pf.trueColour {
		b[3] = 1
	}
	binary.BigEndian.PutUint16(b[4:6], pf.redMax)
	binary.BigEndian.PutUint16(b[6:8], pf.greenMax)
	binary.BigEndian.PutUint16(b[8:10], pf.blueMax)
	b[10], b[11], b[12] = pf.redShift, pf.greenShift, pf.blueShift
	return b
}

func (pf pixelFormat) bytesPerPixel() int { return int(pf.bitsPerPixel) / 8 }

// canReceiveReduced reports whether a client asking for pf can be served
// from reducedFormat: it must be true colour with 8, 16 or 32 bits per pixel,
// and its colours must fit those bits.
func (pf pixelFormat) canReceiveReduced() bool {
	switch pf.bitsPerPixel {
	case 8, 16, 32:
	default:
		return false
	}
	if !pf.trueColour || pf.redMax == 0 || pf.greenMax == 0 || pf.blueMax == 0 {
		return false
	}
	for _, c := range []struct {
		max   uint16
		shift uint8
	}{{pf.redMax, pf.redShift}, {pf.greenMax, pf.greenShift}, {pf.blueMax, pf.blueShift}} {
		if c.shift >= pf.bitsPerPixel || uint64(c.max)<<c.shift >= 1<<pf.bitsPerPixel {
			return false
		}
	}
	return true
}

// cpixel returns which bytes of a pixel ZRLE sends as its compressed pixel: a
// 32-bit true colour pixel whose colours fit in three of its bytes is sent as
// those three bytes, any other pixel whole.
func (pf pixelFormat) cpixel() (offset, length int) {
	n := pf.bytesPerPixel()
	if pf.bitsPerPixel != 32 || pf.depth > 24 || !pf.trueColour {
		return 0, n
	}
	var used uint32
	used |= uint32(pf.redMax) << pf.redShift
	used |= uint32(pf.greenMax) << pf.greenShift
	used |= uint32(pf.blueMax) << pf.blueShift
	switch {
	case used&0xff000000 == 0:
		// The least significant three bytes: first in little endian.
		if pf.bigEndian {
			return 1, 3
		}
		return 0, 3
	case used&0xff == 0:
		if pf.bigEndian {
			return 0, 3
		}
		return 1, 3
	}
	return 0, n
}

// converter turns reducedFormat pixels into a client's pixel format through a
// table, one entry per 16-bit value.
type converter struct {
	size  int // bytes per client pixel
	cOff  int // where the client's ZRLE pixel starts within a pixel
	cSize int // bytes per client ZRLE pixel
	table []byte
}

func newConverter(client pixelFormat) (*converter, error) {
	if !client.canReceiveReduced() {
		return nil, fmt.Errorf("unsupported client pixel format %+v", client)
	}
	size := client.bytesPerPixel()
	cOff, cSize := client.cpixel()
	c := &converter{size: size, cOff: cOff, cSize: cSize, table: make([]byte, 1<<16*size)}
	scale := func(v uint32, from, to uint16) uint32 {
		return (v*uint32(to) + uint32(from)/2) / uint32(from)
	}
	src := reducedFormat
	for v := uint32(0); v < 1<<16; v++ {
		r := scale(v>>src.redShift&uint32(src.redMax), src.redMax, client.redMax)
		g := scale(v>>src.greenShift&uint32(src.greenMax), src.greenMax, client.greenMax)
		b := scale(v>>src.blueShift&uint32(src.blueMax), src.blueMax, client.blueMax)
		out := r<<client.redShift | g<<client.greenShift | b<<client.blueShift
		entry := c.table[int(v)*size : int(v+1)*size]
		switch {
		case size == 1:
			entry[0] = byte(out)
		case size == 2 && client.bigEndian:
			binary.BigEndian.PutUint16(entry, uint16(out))
		case size == 2:
			binary.LittleEndian.PutUint16(entry, uint16(out))
		case client.bigEndian:
			binary.BigEndian.PutUint32(entry, out)
		default:
			binary.LittleEndian.PutUint32(entry, out)
		}
	}
	return c, nil
}

// reducedPixelSize is the size of a reducedFormat pixel, which is little
// endian. ZRLE sends it whole.
const reducedPixelSize = 2

// pixels appends the client form of reducedFormat pixels.
func (c *converter) pixels(dst, src []byte) []byte {
	return c.convert(dst, src, 0, c.size)
}

// cpixels appends the client's ZRLE form of reducedFormat pixels.
func (c *converter) cpixels(dst, src []byte) []byte {
	return c.convert(dst, src, c.cOff, c.cSize)
}

func (c *converter) convert(dst, src []byte, offset, size int) []byte {
	count := len(src) / reducedPixelSize
	start := len(dst)
	dst = slices.Grow(dst, count*size)[:start+count*size]
	out := dst[start:]
	table := c.table
	switch size {
	case 3:
		// noVNC's ZRLE pixels, the bulk of the work.
		for i, o := 0, 0; i < count; i, o = i+1, o+3 {
			e := (int(src[2*i])|int(src[2*i+1])<<8)*c.size + offset
			out[o], out[o+1], out[o+2] = table[e], table[e+1], table[e+2]
		}
	case 4:
		for i, o := 0, 0; i < count; i, o = i+1, o+4 {
			e := (int(src[2*i]) | int(src[2*i+1])<<8) * 4
			out[o], out[o+1], out[o+2], out[o+3] = table[e], table[e+1], table[e+2], table[e+3]
		}
	default:
		for i := 0; i < count; i++ {
			e := (int(src[2*i])|int(src[2*i+1])<<8)*c.size + offset
			copy(out[i*size:i*size+size], table[e:e+size])
		}
	}
	return dst
}
