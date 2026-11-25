package vnc

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"io"
	"math/rand"
	"net"
	"slices"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// noVNCFormat is the pixel format noVNC asks for: 32 bits, little endian,
// red in the lowest byte.
var noVNCFormat = pixelFormat{
	bitsPerPixel: 32, depth: 24, trueColour: true,
	redMax: 255, greenMax: 255, blueMax: 255,
	redShift: 0, greenShift: 8, blueShift: 16,
}

// noVNCEncodings is what noVNC 1.7 sends in SetEncodings, without H.264.
var noVNCEncodings = []int32{1, 7, -260, 16, 21, 5, 2, 6, 0, -26, -254, -223, -224, -258, -261, -308, -309, -312, -313, -307, -1063131698, -316, 0x574d5664, -239}

// rgb16 is a reducedFormat pixel; expand gives its 8-bit channels as the
// converter scales them.
func expand(v uint16) [3]byte {
	r := (uint32(v>>11&31)*255 + 15) / 31
	g := (uint32(v>>5&63)*255 + 31) / 63
	b := (uint32(v&31)*255 + 15) / 31
	return [3]byte{byte(r), byte(g), byte(b)}
}

func TestConverterScalesTo32Bit(t *testing.T) {
	conv, err := newConverter(noVNCFormat)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[uint16][]byte{
		0xffff: {255, 255, 255, 0},
		0xf800: {255, 0, 0, 0},
		0x07e0: {0, 255, 0, 0},
		0x001f: {0, 0, 255, 0},
		0x0000: {0, 0, 0, 0},
	}
	for v, want := range cases {
		src := binary.LittleEndian.AppendUint16(nil, v)
		if got := conv.pixels(nil, src); !bytes.Equal(got, want) {
			t.Errorf("pixel %#04x = %v, want %v", v, got, want)
		}
		if got := conv.cpixels(nil, src); !bytes.Equal(got, want[:3]) {
			t.Errorf("ZRLE pixel %#04x = %v, want %v", v, got, want[:3])
		}
	}

	bigEndian := noVNCFormat
	bigEndian.bigEndian = true
	conv, err = newConverter(bigEndian)
	if err != nil {
		t.Fatal(err)
	}
	if got := conv.pixels(nil, []byte{0x00, 0xf8}); !bytes.Equal(got, []byte{0, 0, 0, 255}) {
		t.Errorf("big endian red = %v", got)
	}
	if got := conv.cpixels(nil, []byte{0x00, 0xf8}); !bytes.Equal(got, []byte{0, 0, 255}) {
		t.Errorf("big endian ZRLE red = %v", got)
	}
}

func TestConverterRefusesFormatsItCannotFill(t *testing.T) {
	colourMap := noVNCFormat
	colourMap.trueColour = false
	tooWide := noVNCFormat
	tooWide.bitsPerPixel, tooWide.depth = 16, 16
	odd := noVNCFormat
	odd.bitsPerPixel = 24
	for _, pf := range []pixelFormat{colourMap, tooWide, odd} {
		if _, err := newConverter(pf); err == nil {
			t.Errorf("%+v accepted", pf)
		}
	}
	if _, err := newConverter(pixelFormat{bitsPerPixel: 8, depth: 6, trueColour: true, redMax: 3, greenMax: 3, blueMax: 3, greenShift: 2, blueShift: 4}); err != nil {
		t.Errorf("noVNC's 8-bit format refused: %v", err)
	}
}

// zrleSubencodings lists one tile kind per tile, in order, for the encoder.
var zrleSubencodings = []string{"raw", "solid", "palette2", "palette5", "rle", "paletteRLE"}

// encodeZRLETile appends a tile of reducedFormat pixels in the given kind.
func encodeZRLETile(dst []byte, kind string, pixels []uint16, w int) []byte {
	px := func(v uint16) []byte { return binary.LittleEndian.AppendUint16(nil, v) }
	palette := func() []uint16 {
		var p []uint16
		for _, v := range pixels {
			if !slices.Contains(p, v) {
				p = append(p, v)
			}
		}
		return p
	}
	run := func(n int) []byte {
		var b []byte
		for n--; n >= 255; n -= 255 {
			b = append(b, 255)
		}
		return append(b, byte(n))
	}
	switch kind {
	case "raw":
		dst = append(dst, 0)
		for _, v := range pixels {
			dst = append(dst, px(v)...)
		}
	case "solid":
		dst = append(dst, 1)
		dst = append(dst, px(pixels[0])...)
	case "palette2", "palette5":
		p := palette()
		dst = append(dst, byte(len(p)))
		for _, v := range p {
			dst = append(dst, px(v)...)
		}
		bits := 1
		if len(p) > 4 {
			bits = 4
		} else if len(p) > 2 {
			bits = 2
		}
		h := len(pixels) / w
		for y := 0; y < h; y++ {
			var acc, n int
			for x := 0; x < w; x++ {
				acc = acc<<bits | slices.Index(p, pixels[y*w+x])
				n += bits
				if n == 8 {
					dst = append(dst, byte(acc))
					acc, n = 0, 0
				}
			}
			if n > 0 {
				dst = append(dst, byte(acc<<(8-n)))
			}
		}
	case "rle":
		dst = append(dst, 128)
		for i := 0; i < len(pixels); {
			j := i
			for j < len(pixels) && pixels[j] == pixels[i] {
				j++
			}
			dst = append(dst, px(pixels[i])...)
			dst = append(dst, run(j-i)...)
			i = j
		}
	case "paletteRLE":
		p := palette()
		dst = append(dst, byte(128+len(p)))
		for _, v := range p {
			dst = append(dst, px(v)...)
		}
		for i := 0; i < len(pixels); {
			j := i
			for j < len(pixels) && pixels[j] == pixels[i] {
				j++
			}
			index := byte(slices.Index(p, pixels[i]))
			if j-i == 1 {
				dst = append(dst, index)
			} else {
				dst = append(dst, index|128)
				dst = append(dst, run(j-i)...)
			}
			i = j
		}
	}
	return dst
}

// screenPixels makes reducedFormat pixels that suit every tile kind: tile
// kinds that need few colours get few.
func screenPixels(w, h int, rng *rand.Rand) []uint16 {
	pixels := make([]uint16, w*h)
	for i := range pixels {
		x, y := i%w, i/w
		tile := (y/zrleTileSize*((w+zrleTileSize-1)/zrleTileSize) + x/zrleTileSize) % len(zrleSubencodings)
		switch zrleSubencodings[tile] {
		case "raw":
			pixels[i] = uint16(rng.Intn(1 << 16))
		case "solid":
			pixels[i] = 0x1234
		case "palette2":
			pixels[i] = []uint16{0xf800, 0x001f}[(x+y)%2]
		case "palette5":
			pixels[i] = []uint16{1, 2, 3, 4, 5}[(x*3+y)%5]
		case "rle":
			pixels[i] = uint16(y * 300)
		case "paletteRLE":
			pixels[i] = []uint16{0x07e0, 0xffff, 0x8410}[(x/7+y)%3]
		}
	}
	return pixels
}

// zrleKind picks the tile kind that suits a tile's pixels, so each of the
// test screen's tile patterns gets its own kind.
func zrleKind(tile []uint16) string {
	var colours []uint16
	runs := 0
	for i, v := range tile {
		if !slices.Contains(colours, v) {
			colours = append(colours, v)
		}
		if i == 0 || tile[i-1] != v {
			runs++
		}
	}
	switch {
	case len(colours) == 1:
		return "solid"
	case len(colours) == 2:
		return "palette2"
	case len(colours) <= 4:
		return "paletteRLE"
	case len(colours) <= 16:
		return "palette5"
	case runs < len(tile)/4:
		return "rle"
	}
	return "raw"
}

// encodeZRLE returns the tile data of a rectangle, before compression.
func encodeZRLE(pixels []uint16, w, h int) []byte {
	var out []byte
	for ty, row := 0, 0; ty < h; ty += zrleTileSize {
		for tx := 0; tx < w; tx, row = tx+zrleTileSize, row+1 {
			tw, th := min(zrleTileSize, w-tx), min(zrleTileSize, h-ty)
			tile := make([]uint16, 0, tw*th)
			for y := ty; y < ty+th; y++ {
				tile = append(tile, pixels[y*w+tx:y*w+tx+tw]...)
			}
			out = encodeZRLETile(out, zrleKind(tile), tile, tw)
		}
	}
	return out
}

// fakeMac is the server side of a session after ClientInit: it checks what the
// bridge asks for and sends one update with every converted encoding.
type fakeMac struct {
	t        *testing.T
	pixels   []uint16
	w, h     int
	asked    chan pixelFormat
	encs     chan []int32
	finished chan struct{}
}

func (m *fakeMac) serve(conn net.Conn) error {
	defer close(m.finished)
	init := binary.BigEndian.AppendUint16(nil, uint16(m.w))
	init = binary.BigEndian.AppendUint16(init, uint16(m.h))
	init = append(init, noVNCFormat.bytes()...)
	init = binary.BigEndian.AppendUint32(init, 3)
	init = append(init, "mac"...)
	if _, err := conn.Write(init); err != nil {
		return err
	}
	in := bufio.NewReader(conn)
	read := func(n int) []byte {
		b := make([]byte, n)
		if _, err := io.ReadFull(in, b); err != nil {
			m.t.Errorf("server read: %v", err)
		}
		return b
	}
	if kind := read(1)[0]; kind != msgSetPixelFormat {
		m.t.Fatalf("first message %d", kind)
	}
	m.asked <- parsePixelFormat(read(19)[3:])
	if kind := read(1)[0]; kind != msgSetEncodings {
		m.t.Fatalf("second message %d", kind)
	}
	count := int(binary.BigEndian.Uint16(read(3)[1:]))
	list := read(4 * count)
	var encs []int32
	for i := 0; i < count; i++ {
		encs = append(encs, int32(binary.BigEndian.Uint32(list[4*i:])))
	}
	m.encs <- encs
	if kind := read(1)[0]; kind != msgUpdateRequest {
		m.t.Fatalf("third message %d", kind)
	}
	read(9)

	var zrleData, zlibData bytes.Buffer
	zrleWriter := zlib.NewWriter(&zrleData)
	zlibWriter := zlib.NewWriter(&zlibData)
	rect := func(x, y, w, h int, enc int32) []byte {
		b := binary.BigEndian.AppendUint16(nil, uint16(x))
		b = binary.BigEndian.AppendUint16(b, uint16(y))
		b = binary.BigEndian.AppendUint16(b, uint16(w))
		b = binary.BigEndian.AppendUint16(b, uint16(h))
		return binary.BigEndian.AppendUint32(b, uint32(enc))
	}
	sub := func(x, y, w, h int) []uint16 {
		var out []uint16
		for row := y; row < y+h; row++ {
			out = append(out, m.pixels[row*m.w+x:row*m.w+x+w]...)
		}
		return out
	}
	le := func(px []uint16) []byte {
		var b []byte
		for _, v := range px {
			b = binary.LittleEndian.AppendUint16(b, v)
		}
		return b
	}
	compressed := func(w *zlib.Writer, data *bytes.Buffer, raw []byte) []byte {
		_, _ = w.Write(raw)
		_ = w.Flush()
		out := binary.BigEndian.AppendUint32(nil, uint32(data.Len()))
		out = append(out, data.Bytes()...)
		data.Reset()
		return out
	}

	var update []byte
	update = append(update, msgFramebufferUpdate, 0, 0, 8)
	// The whole screen in ZRLE, then a part again on the same zlib stream.
	update = append(update, rect(0, 0, m.w, m.h, encodingZRLE)...)
	update = append(update, compressed(zrleWriter, &zrleData, encodeZRLE(m.pixels, m.w, m.h))...)
	update = append(update, rect(64, 0, 70, 66, encodingZRLE)...)
	update = append(update, compressed(zrleWriter, &zrleData, encodeZRLE(sub(64, 0, 70, 66), 70, 66))...)
	update = append(update, rect(1, 2, 5, 3, encodingRaw)...)
	update = append(update, le(sub(1, 2, 5, 3))...)
	update = append(update, rect(10, 10, 4, 4, encodingZlib)...)
	update = append(update, compressed(zlibWriter, &zlibData, le(sub(10, 10, 4, 4)))...)
	update = append(update, rect(20, 20, 4, 4, encodingZlib)...)
	update = append(update, compressed(zlibWriter, &zlibData, le(sub(20, 20, 4, 4)))...)
	update = append(update, rect(5, 6, 7, 8, encodingCopyRect)...)
	update = append(update, 0, 1, 0, 2)
	update = append(update, rect(2, 3, 9, 2, encodingCursor)...)
	update = append(update, le(sub(0, 0, 9, 2))...)
	update = append(update, 0xff, 0x80, 0x7f, 0x00)
	update = append(update, rect(0, 0, 0, 0, encodingDesktopName)...)
	update = binary.BigEndian.AppendUint32(update, 4)
	update = append(update, "mac2"...)
	update = append(update, msgBell)
	update = append(update, msgServerCutText, 0, 0, 0, 0, 0, 0, 2, 'h', 'i')
	_, err := conn.Write(update)
	return err
}

// decodedClient reads what the bridge sends noVNC and decodes it into pixels.
type decodedClient struct {
	t      *testing.T
	in     *bufio.Reader
	zrle   zlibInput
	screen map[[2]int][]byte
}

func (c *decodedClient) read(n int) []byte {
	b := make([]byte, n)
	if _, err := io.ReadFull(c.in, b); err != nil {
		c.t.Fatalf("client read %d: %v", n, err)
	}
	return b
}

func (c *decodedClient) put(x, y int, px []byte) {
	if c.screen == nil {
		return
	}
	c.screen[[2]int{x, y}] = slices.Clone(px)
}

func (c *decodedClient) zrleRect(x0, y0, w, h int) {
	data := c.read(int(binary.BigEndian.Uint32(c.read(4))))
	if err := c.zrle.feed(data); err != nil {
		c.t.Fatal(err)
	}
	must := func(b []byte, err error) []byte {
		if err != nil {
			c.t.Fatal(err)
		}
		return b
	}
	cpixel := func() []byte { return append(slices.Clone(must(c.zrle.read(3))), 0) }
	for ty := y0; ty < y0+h; ty += zrleTileSize {
		for tx := x0; tx < x0+w; tx += zrleTileSize {
			tw, th := min(zrleTileSize, x0+w-tx), min(zrleTileSize, y0+h-ty)
			var tile [][]byte
			sub := must(c.zrle.read(1))[0]
			runOf := func() int {
				n := 1
				for {
					b := must(c.zrle.read(1))[0]
					n += int(b)
					if b != 255 {
						return n
					}
				}
			}
			switch {
			case sub == 0:
				for i := 0; i < tw*th; i++ {
					tile = append(tile, cpixel())
				}
			case sub == 1:
				p := cpixel()
				for i := 0; i < tw*th; i++ {
					tile = append(tile, p)
				}
			case sub <= 16:
				var palette [][]byte
				for i := 0; i < int(sub); i++ {
					palette = append(palette, cpixel())
				}
				bits := 4
				if sub == 2 {
					bits = 1
				} else if sub <= 4 {
					bits = 2
				}
				for y := 0; y < th; y++ {
					row := must(c.zrle.read((tw*bits + 7) / 8))
					for x := 0; x < tw; x++ {
						bit := x * bits
						index := int(row[bit/8]) >> (8 - bits - bit%8) & (1<<bits - 1)
						tile = append(tile, palette[index])
					}
				}
			case sub == 128:
				for len(tile) < tw*th {
					p := cpixel()
					for n := runOf(); n > 0; n-- {
						tile = append(tile, p)
					}
				}
			default:
				var palette [][]byte
				for i := 0; i < int(sub)-128; i++ {
					palette = append(palette, cpixel())
				}
				for len(tile) < tw*th {
					index := must(c.zrle.read(1))[0]
					n := 1
					if index&128 != 0 {
						n = runOf()
					}
					for ; n > 0; n-- {
						tile = append(tile, palette[index&127])
					}
				}
			}
			for i, p := range tile {
				c.put(tx+i%tw, ty+i/tw, p)
			}
		}
	}
}

func noVNCHandshake(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	if got := string(readExactly(t, conn, versionLength)); got != "RFB 003.008\n" {
		t.Fatalf("banner %q", got)
	}
	send(t, conn, []byte("RFB 003.008\n"))
	readExactly(t, conn, 2)
	send(t, conn, []byte{securityNone, 1})
	readExactly(t, conn, 4)
}

func noVNCInit(t *testing.T, conn *websocket.Conn, pf pixelFormat) {
	t.Helper()
	msg := append([]byte{msgSetPixelFormat, 0, 0, 0}, pf.bytes()...)
	msg = append(msg, msgSetEncodings, 0)
	msg = binary.BigEndian.AppendUint16(msg, uint16(len(noVNCEncodings)))
	for _, e := range noVNCEncodings {
		msg = binary.BigEndian.AppendUint32(msg, uint32(e))
	}
	msg = append(msg, msgUpdateRequest, 0, 0, 0, 0, 0, 1, 0, 1, 0)
	send(t, conn, msg)
}

func macTarget(mac *fakeMac, version string) Target {
	return Target{Dial: func(context.Context) (net.Conn, error) {
		client, server := net.Pipe()
		go func() {
			_, _ = fakeServer{version: version, types: []byte{securityNone}, session: mac.serve}.serve(server)
		}()
		return client, nil
	}}
}

func newFakeMac(t *testing.T) *fakeMac {
	w, h := 150, 140
	return &fakeMac{t: t, w: w, h: h, pixels: screenPixels(w, h, rand.New(rand.NewSource(1))),
		asked: make(chan pixelFormat, 1), encs: make(chan []int32, 1), finished: make(chan struct{})}
}

func TestBridgeReducesColourForMac(t *testing.T) {
	bridge := NewBridge(false)
	defer bridge.Close()
	mac := newFakeMac(t)
	// The screen uses every tile kind.
	kinds := map[string]bool{}
	for ty := 0; ty < mac.h; ty += zrleTileSize {
		for tx := 0; tx < mac.w; tx += zrleTileSize {
			var tile []uint16
			for y := ty; y < min(ty+zrleTileSize, mac.h); y++ {
				tile = append(tile, mac.pixels[y*mac.w+tx:y*mac.w+min(tx+zrleTileSize, mac.w)]...)
			}
			kinds[zrleKind(tile)] = true
		}
	}
	if len(kinds) != len(zrleSubencodings) {
		t.Fatalf("test screen uses only %v", kinds)
	}
	session := open(t, bridge, macTarget(mac, "RFB 003.889\n"))
	if !session.Apple || !session.ReducedColor {
		t.Fatalf("session %+v", session)
	}
	conn, _, err := dial(t, session.ProxyAddress, webviewOrigin)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	noVNCHandshake(t, conn)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	client := &decodedClient{t: t, in: bufio.NewReader(&messageReader{conn: conn}), screen: map[[2]int][]byte{}}
	init := client.read(serverInitFixedLength)
	if w := binary.BigEndian.Uint16(init); int(w) != mac.w {
		t.Fatalf("ServerInit width %d", w)
	}
	client.read(int(binary.BigEndian.Uint32(init[20:])))
	noVNCInit(t, conn, noVNCFormat)

	if asked := <-mac.asked; asked != reducedFormat {
		t.Errorf("server asked for %+v", asked)
	}
	if encs := <-mac.encs; !slices.Equal(encs, []int32{1, 16, 6, 0, -26, -254, -223, -224, -307, -1063131698, -239}) {
		t.Errorf("server offered %v", encs)
	}

	header := client.read(4)
	if header[0] != msgFramebufferUpdate || binary.BigEndian.Uint16(header[2:]) != 8 {
		t.Fatalf("update header %v", header)
	}
	want := func(x, y int) []byte {
		c := expand(mac.pixels[y*mac.w+x])
		return []byte{c[0], c[1], c[2], 0}
	}
	check := func(what string, x0, y0, w, h int) {
		t.Helper()
		for y := y0; y < y0+h; y++ {
			for x := x0; x < x0+w; x++ {
				if got := client.screen[[2]int{x, y}]; !bytes.Equal(got, want(x, y)) {
					t.Fatalf("%s pixel %d,%d = %v, want %v", what, x, y, got, want(x, y))
				}
			}
		}
	}
	rect := func() (int, int, int, int, int32) {
		b := client.read(12)
		return int(binary.BigEndian.Uint16(b)), int(binary.BigEndian.Uint16(b[2:])), int(binary.BigEndian.Uint16(b[4:])), int(binary.BigEndian.Uint16(b[6:])), int32(binary.BigEndian.Uint32(b[8:]))
	}
	for i := 0; i < 2; i++ {
		x, y, w, h, enc := rect()
		if enc != encodingZRLE {
			t.Fatalf("rect %d encoding %d", i, enc)
		}
		client.zrleRect(x, y, w, h)
		check("ZRLE", x, y, w, h)
	}
	for _, place := range [][4]int{{1, 2, 5, 3}, {10, 10, 4, 4}, {20, 20, 4, 4}} {
		x, y, w, h, enc := rect()
		if enc != encodingRaw || [4]int{x, y, w, h} != place {
			t.Fatalf("raw rect %d,%d %dx%d encoding %d", x, y, w, h, enc)
		}
		raw := client.read(w * h * 4)
		for i := 0; i < w*h; i++ {
			client.put(x+i%w, y+i/w, raw[4*i:4*i+4])
		}
		check("Raw", x, y, w, h)
	}
	if _, _, _, _, enc := rect(); enc != encodingCopyRect || !bytes.Equal(client.read(4), []byte{0, 1, 0, 2}) {
		t.Fatalf("CopyRect encoding %d", enc)
	}
	if _, _, w, h, enc := rect(); enc != encodingCursor || w != 9 || h != 2 {
		t.Fatalf("cursor %dx%d encoding %d", w, h, enc)
	}
	cursor := client.read(9 * 2 * 4)
	for i := 0; i < 18; i++ {
		if !bytes.Equal(cursor[4*i:4*i+4], want(i%9, i/9)) {
			t.Fatalf("cursor pixel %d = %v", i, cursor[4*i:4*i+4])
		}
	}
	if mask := client.read(4); !bytes.Equal(mask, []byte{0xff, 0x80, 0x7f, 0x00}) {
		t.Errorf("cursor mask %v", mask)
	}
	if _, _, _, _, enc := rect(); enc != encodingDesktopName {
		t.Fatalf("name encoding %d", enc)
	}
	if name := client.read(8); string(name[4:]) != "mac2" {
		t.Errorf("desktop name %q", name)
	}
	if rest := client.read(11); !bytes.Equal(rest, []byte{msgBell, msgServerCutText, 0, 0, 0, 0, 0, 0, 2, 'h', 'i'}) {
		t.Errorf("bell and clipboard %v", rest)
	}
}

// relayedSession opens a session, runs the client handshake and init with pf,
// and returns what the server was asked for.
func relayedSession(t *testing.T, target Target, mac *fakeMac, pf pixelFormat) (Session, pixelFormat, []int32) {
	t.Helper()
	bridge := NewBridge(false)
	t.Cleanup(bridge.Close)
	session := open(t, bridge, target)
	conn, _, err := dial(t, session.ProxyAddress, webviewOrigin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	noVNCHandshake(t, conn)
	readExactly(t, conn, serverInitFixedLength+3)
	noVNCInit(t, conn, pf)
	asked, encs := <-mac.asked, <-mac.encs
	// The unconverted stream reaches the client unchanged.
	if got := readExactly(t, conn, 4); got[0] != msgFramebufferUpdate || got[3] != 8 {
		t.Errorf("update header %v", got)
	}
	return session, asked, encs
}

func TestBridgeKeepsFullColourWhenAsked(t *testing.T) {
	mac := newFakeMac(t)
	target := macTarget(mac, "RFB 003.889\n")
	target.FullColor = true
	session, asked, encs := relayedSession(t, target, mac, noVNCFormat)
	if !session.Apple || session.ReducedColor {
		t.Errorf("session %+v", session)
	}
	if asked != noVNCFormat || !slices.Equal(encs, noVNCEncodings) {
		t.Errorf("server asked for %+v with %v", asked, encs)
	}
}

func TestBridgeKeepsOtherServersUnchanged(t *testing.T) {
	mac := newFakeMac(t)
	session, asked, encs := relayedSession(t, macTarget(mac, "RFB 003.008\n"), mac, noVNCFormat)
	if session.Apple || session.ReducedColor {
		t.Errorf("session %+v", session)
	}
	if asked != noVNCFormat || !slices.Equal(encs, noVNCEncodings) {
		t.Errorf("server asked for %+v with %v", asked, encs)
	}
}

func TestBridgeLeavesFormatsItCannotConvert(t *testing.T) {
	mac := newFakeMac(t)
	colourMap := pixelFormat{bitsPerPixel: 8, depth: 8}
	_, asked, encs := relayedSession(t, macTarget(mac, "RFB 003.889\n"), mac, colourMap)
	if asked != colourMap || !slices.Equal(encs, noVNCEncodings) {
		t.Errorf("server asked for %+v with %v", asked, encs)
	}
}

func TestBridgeEndsMacSessionWhenClientLeavesEarly(t *testing.T) {
	bridge := NewBridge(false)
	defer bridge.Close()
	serverDone := make(chan struct{})
	target := Target{Dial: func(context.Context) (net.Conn, error) {
		client, server := net.Pipe()
		session := func(conn net.Conn) error {
			defer close(serverDone)
			init := binary.BigEndian.AppendUint16(nil, 64)
			init = binary.BigEndian.AppendUint16(init, 48)
			init = append(init, noVNCFormat.bytes()...)
			init = binary.BigEndian.AppendUint32(init, 0)
			if _, err := conn.Write(init); err != nil {
				return err
			}
			_, err := io.Copy(io.Discard, conn)
			return err
		}
		go func() {
			_, _ = fakeServer{version: "RFB 003.889\n", types: []byte{securityNone}, session: session}.serve(server)
		}()
		return client, nil
	}}
	session := open(t, bridge, target)
	conn, _, err := dial(t, session.ProxyAddress, webviewOrigin)
	if err != nil {
		t.Fatal(err)
	}
	noVNCHandshake(t, conn)
	readExactly(t, conn, serverInitFixedLength)
	// The client leaves before it sends a pixel format.
	conn.Close()
	select {
	case <-serverDone:
	case <-time.After(3 * time.Second):
		t.Fatal("the server connection stayed open")
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		bridge.mu.Lock()
		active := len(bridge.active)
		bridge.mu.Unlock()
		if active == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the session stayed active")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestTranscoderRejectsRectanglesLargerThanTheScreen(t *testing.T) {
	conv, err := newConverter(noVNCFormat)
	if err != nil {
		t.Fatal(err)
	}
	reducer := newColourReducer()
	reducer.decide(conv)
	stream := binary.BigEndian.AppendUint16(nil, 64)
	stream = binary.BigEndian.AppendUint16(stream, 48)
	stream = append(stream, noVNCFormat.bytes()...)
	// A long name is cut, and its excess skipped.
	stream = binary.BigEndian.AppendUint32(stream, maxReasonLength+3)
	stream = append(stream, bytes.Repeat([]byte{'n'}, maxReasonLength+3)...)
	stream = append(stream, msgBell, msgFramebufferUpdate, 0, 0, 1)
	stream = append(stream, 0, 0, 0, 0, 0xff, 0xff, 0xff, 0xff, 0, 0, 0, byte(encodingZlib))
	var out []byte
	err = relayServerReducing(context.Background(), bytes.NewReader(stream), func(b []byte) error {
		out = append(out, b...)
		return nil
	}, reducer)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("larger than the screen")) {
		t.Fatalf("err = %v", err)
	}
	if got := binary.BigEndian.Uint32(out[20:]); got != maxReasonLength {
		t.Fatalf("name length %d", got)
	}
	// Reaching the rectangle shows the excess of the name was skipped.
	if len(out) != serverInitFixedLength+maxReasonLength {
		t.Fatalf("sent %d bytes", len(out))
	}
}
