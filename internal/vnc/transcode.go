package vnc

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"slices"
	"sync"
)

// macOS Screen Sharing sends the Retina screen in full and redraws large parts
// of it for small changes, so the amount of data limits how fast the screen
// follows input. For such servers the bridge asks for 16-bit colour, which
// halves the data, and converts it back to the pixel format the web client
// asked for, because noVNC cannot show 16-bit colour itself.
//
// The conversion needs to understand every message, so it is limited to what
// the server and noVNC use: the encodings the client may ask for are cut down
// to the ones converted here, and anything else ends the session.

// RFB encodings and pseudo-encodings the conversion handles.
const (
	encodingRaw         int32 = 0
	encodingCopyRect    int32 = 1
	encodingZlib        int32 = 6
	encodingZRLE        int32 = 16
	encodingCursor      int32 = -239
	encodingDesktopSize int32 = -223
	encodingLastRect    int32 = -224
	encodingDesktopName int32 = -307
	// encodingExtendedClipboard changes only the clipboard messages, whose
	// negative lengths the relays read.
	encodingExtendedClipboard int32 = -1063131698
	// Ranges of the quality and compression level pseudo-encodings, which
	// carry no data.
	encodingQualityFirst  int32 = -32
	encodingQualityLast   int32 = -23
	encodingCompressFirst int32 = -256
	encodingCompressLast  int32 = -247
)

// Client and server message types.
const (
	msgSetPixelFormat     = 0
	msgSetEncodings       = 2
	msgUpdateRequest      = 3
	msgKeyEvent           = 4
	msgPointerEvent       = 5
	msgClientCutText      = 6
	msgFramebufferUpdate  = 0
	msgSetColourMap       = 1
	msgBell               = 2
	msgServerCutText      = 3
	zrleTileSize          = 64
	maxRectData           = 64 << 20
	maxCutText            = 16 << 20
	transcodeFlushSize    = 256 << 10
	transcodeReadBuffer   = 256 << 10
	serverInitFixedLength = 24
	// maxCursorPixels is far above any real pointer, even an enlarged one.
	maxCursorPixels = 512 * 512
)

// zrleOutputLevel is the fastest compression: the loopback has bandwidth to
// spare, but not for an uncompressed Retina screen.
const zrleOutputLevel = zlib.BestSpeed

func convertible(encoding int32) bool {
	switch encoding {
	case encodingRaw, encodingCopyRect, encodingZlib, encodingZRLE, encodingCursor,
		encodingDesktopSize, encodingLastRect, encodingDesktopName, encodingExtendedClipboard:
		return true
	}
	return encoding >= encodingQualityFirst && encoding <= encodingQualityLast ||
		encoding >= encodingCompressFirst && encoding <= encodingCompressLast
}

// colourReducer carries what the client-side relay learns to the server side:
// whether the session is converted, and into which pixel format.
type colourReducer struct {
	decided chan struct{}
	once    sync.Once

	mu        sync.Mutex
	active    bool
	converter *converter
}

func newColourReducer() *colourReducer {
	return &colourReducer{decided: make(chan struct{})}
}

// decide fixes whether the session is converted. Only the first call counts.
func (r *colourReducer) decide(conv *converter) {
	r.once.Do(func() {
		r.mu.Lock()
		r.active = conv != nil
		r.converter = conv
		r.mu.Unlock()
		close(r.decided)
	})
}

// mayBeActive is false only once the session is known to stay unconverted.
func (r *colourReducer) mayBeActive() bool {
	select {
	case <-r.decided:
		active, _ := r.state()
		return active
	default:
		return true
	}
}

func (r *colourReducer) state() (bool, *converter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.active, r.converter
}

// setConverter replaces the converter of an active session after the client
// changed its pixel format again.
func (r *colourReducer) setConverter(conv *converter) {
	r.mu.Lock()
	r.converter = conv
	r.mu.Unlock()
}

// relayClientReducing copies the client's messages to the server. The first
// pixel format the client asks for decides the session: one that 16-bit colour
// can be converted into is replaced by reducedFormat, and the client's
// encodings are then cut down to the converted ones. Any other format, or an
// update request before a pixel format, leaves the session unconverted.
func relayClientReducing(client io.Reader, server io.Writer, reducer *colourReducer) error {
	// A client that leaves before choosing a format must not keep the server
	// side waiting for the decision.
	defer reducer.decide(nil)
	var out bytes.Buffer
	send := func() error {
		if out.Len() == 0 {
			return nil
		}
		_, err := server.Write(out.Bytes())
		out.Reset()
		return err
	}
	// Messages that arrived together go out together; whatever is pending
	// goes out before the relay waits for more.
	in := bufio.NewReaderSize(&flushingReader{r: client, flush: send}, 64<<10)
	read := func(n int) ([]byte, error) {
		buf := make([]byte, n)
		_, err := io.ReadFull(in, buf)
		return buf, err
	}
	// The client's first message is its one-byte ClientInit.
	init, err := read(1)
	if err != nil {
		return err
	}
	out.Write(init)
	for {
		kind, err := in.ReadByte()
		if err != nil {
			return err
		}
		out.WriteByte(kind)
		switch kind {
		case msgSetPixelFormat:
			body, err := read(3 + pixelFormatLength)
			if err != nil {
				return err
			}
			if err := reducePixelFormat(body, reducer); err != nil {
				return err
			}
			out.Write(body)
		case msgSetEncodings:
			header, err := read(3)
			if err != nil {
				return err
			}
			list, err := read(4 * int(binary.BigEndian.Uint16(header[1:])))
			if err != nil {
				return err
			}
			// Cut down unless the session is known to stay unconverted, in
			// case the encodings come before the pixel format.
			if reducer.mayBeActive() {
				list = keepConvertible(list)
				binary.BigEndian.PutUint16(header[1:], uint16(len(list)/4))
			}
			out.Write(header)
			out.Write(list)
		case msgUpdateRequest, msgKeyEvent, msgPointerEvent:
			// An update request before any pixel format gets the server's
			// own format, which is not converted.
			if kind == msgUpdateRequest {
				reducer.decide(nil)
			}
			body, err := read(fixedClientMessageSize(kind))
			if err != nil {
				return err
			}
			out.Write(body)
		case msgClientCutText:
			header, err := read(7)
			if err != nil {
				return err
			}
			length, err := cutTextLength(header[3:])
			if err != nil {
				return err
			}
			body, err := read(length)
			if err != nil {
				return err
			}
			out.Write(header)
			out.Write(body)
		default:
			if active, _ := reducer.state(); active {
				return fmt.Errorf("unsupported VNC client message %d", kind)
			}
			// Unconverted sessions forward the rest as it comes.
			reducer.decide(nil)
			if err := send(); err != nil {
				return err
			}
			_, err := io.Copy(server, in)
			return err
		}
	}
}

// fixedClientMessageSize is the size, after the type byte, of the client
// messages that have one.
func fixedClientMessageSize(kind byte) int {
	switch kind {
	case msgUpdateRequest:
		return 9
	case msgKeyEvent:
		return 7
	}
	return 5 // msgPointerEvent
}

// flushingReader runs flush before every read of its source, so output
// waiting for the end of a batch leaves as soon as the input runs dry.
type flushingReader struct {
	r     io.Reader
	flush func() error
}

func (f *flushingReader) Read(p []byte) (int, error) {
	if err := f.flush(); err != nil {
		return 0, err
	}
	return f.r.Read(p)
}

// reducePixelFormat rewrites a SetPixelFormat body for the server.
func reducePixelFormat(body []byte, reducer *colourReducer) error {
	client := parsePixelFormat(body[3:])
	select {
	case <-reducer.decided:
		active, _ := reducer.state()
		if !active {
			return nil
		}
		conv, err := newConverter(client)
		if err != nil {
			// The server already sends 16-bit colour, which this client
			// could not take.
			return err
		}
		reducer.setConverter(conv)
	default:
		conv, err := newConverter(client)
		if err != nil {
			reducer.decide(nil)
			return nil
		}
		reducer.decide(conv)
	}
	copy(body[3:], reducedFormat.bytes())
	return nil
}

func keepConvertible(list []byte) []byte {
	kept := make([]byte, 0, len(list))
	for i := 0; i+4 <= len(list); i += 4 {
		if convertible(int32(binary.BigEndian.Uint32(list[i:]))) {
			kept = append(kept, list[i:i+4]...)
		}
	}
	return kept
}

// cutTextLength reads a cut text length. A negative one marks the extended
// clipboard format, whose data is as long as its absolute value.
func cutTextLength(b []byte) (int, error) {
	length := int64(int32(binary.BigEndian.Uint32(b)))
	if length < 0 {
		length = -length
	}
	if length > maxCutText {
		return 0, fmt.Errorf("VNC clipboard text of %d bytes is too long", length)
	}
	return int(length), nil
}

// zlibQueue feeds a long-lived zlib stream with the data of one rectangle at a
// time. The stream may leave bytes unread at the end of a rectangle; they stay
// for the next one. It implements io.ByteReader so the decompressor never
// reads ahead.
type zlibQueue struct {
	data []byte
}

func (q *zlibQueue) Read(p []byte) (int, error) {
	if len(q.data) == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	n := copy(p, q.data)
	q.data = q.data[n:]
	return n, nil
}

func (q *zlibQueue) ReadByte() (byte, error) {
	if len(q.data) == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	b := q.data[0]
	q.data = q.data[1:]
	return b, nil
}

func (q *zlibQueue) add(data []byte) {
	q.data = append(q.data, data...)
}

// zlibInput is one of the server's zlib streams. ZRLE and Zlib each have one
// for the whole connection. RFB requires a rectangle's data to end where the
// server flushed its stream, so a rectangle never needs bytes of the next one;
// a server that breaks this ends the session, because the decompressor keeps
// its first error.
type zlibInput struct {
	queue  zlibQueue
	reader io.ReadCloser
	buf    []byte
}

func (z *zlibInput) feed(data []byte) error {
	z.queue.add(data)
	if z.reader == nil {
		reader, err := zlib.NewReader(&z.queue)
		if err != nil {
			return fmt.Errorf("starting the VNC zlib stream: %w", err)
		}
		z.reader = reader
	}
	return nil
}

// read returns the next n bytes, valid until the next call.
func (z *zlibInput) read(n int) ([]byte, error) {
	if cap(z.buf) < n {
		z.buf = make([]byte, n)
	}
	buf := z.buf[:n]
	if _, err := io.ReadFull(z.reader, buf); err != nil {
		return nil, fmt.Errorf("decompressing VNC data: %w", err)
	}
	return buf, nil
}

func (z *zlibInput) readByte() (byte, error) {
	b, err := z.read(1)
	if err != nil {
		return 0, err
	}
	return b[0], nil
}

// serverTranscoder converts what a server sends in reducedFormat.
type serverTranscoder struct {
	in    *bufio.Reader
	out   bytes.Buffer
	flush func([]byte) error
	conv  *converter
	// maxPixels bounds a rectangle by the framebuffer, so a server cannot make
	// the app allocate by sizes it makes up.
	maxPixels int

	zrleIn   zlibInput
	zlibIn   zlibInput
	zrleOut  *zlib.Writer
	zrleData bytes.Buffer
}

func (t *serverTranscoder) read(n int) ([]byte, error) {
	buf := make([]byte, n)
	_, err := io.ReadFull(t.in, buf)
	return buf, err
}

func (t *serverTranscoder) send() error {
	if t.out.Len() == 0 {
		return nil
	}
	err := t.flush(t.out.Bytes())
	t.out.Reset()
	return err
}

// relayServerReducing passes the server's ServerInit on, waits until the
// client side has decided whether the session is converted, and then either
// converts every message or copies the stream unchanged.
func relayServerReducing(ctx context.Context, server io.Reader, flush func([]byte) error, reducer *colourReducer) error {
	t := &serverTranscoder{flush: flush}
	// Converted output goes out whenever the relay is about to wait for the
	// server, so nothing finished waits for the next message.
	t.in = bufio.NewReaderSize(&flushingReader{r: server, flush: t.send}, transcodeReadBuffer)
	init, err := t.read(serverInitFixedLength - 4)
	if err != nil {
		return err
	}
	t.setFramebuffer(init)
	t.out.Write(init)
	if err := t.name(); err != nil {
		return err
	}
	if err := t.send(); err != nil {
		return err
	}

	select {
	case <-reducer.decided:
	case <-ctx.Done():
		return nil
	}
	if active, _ := reducer.state(); !active {
		return copyToClient(t.in, flush)
	}
	for {
		kind, err := t.in.ReadByte()
		if err != nil {
			return err
		}
		switch kind {
		case msgFramebufferUpdate:
			_, t.conv = reducer.state()
			err = t.update()
		case msgBell:
			t.out.WriteByte(kind)
		case msgServerCutText:
			err = t.cutText()
		case msgSetColourMap:
			err = errors.New("the VNC server sent a colour map for a true colour session")
		default:
			err = fmt.Errorf("unsupported VNC server message %d", kind)
		}
		if err != nil {
			return err
		}
		if t.out.Len() >= transcodeFlushSize {
			if err := t.send(); err != nil {
				return err
			}
		}
	}
}

// setFramebuffer takes the size from a ServerInit or DesktopSize header,
// whose width and height come first.
func (t *serverTranscoder) setFramebuffer(header []byte) {
	t.maxPixels = int(binary.BigEndian.Uint16(header)) * int(binary.BigEndian.Uint16(header[2:]))
}

// name passes on a length-prefixed name, cut to maxReasonLength.
func (t *serverTranscoder) name() error {
	length, err := t.read(4)
	if err != nil {
		return err
	}
	n := binary.BigEndian.Uint32(length)
	keep := min(n, maxReasonLength)
	name, err := t.read(int(keep))
	if err != nil {
		return err
	}
	if _, err := io.CopyN(io.Discard, t.in, int64(n-keep)); err != nil {
		return err
	}
	t.out.Write(binary.BigEndian.AppendUint32(nil, keep))
	t.out.Write(name)
	return nil
}

func copyToClient(in io.Reader, flush func([]byte) error) error {
	buffer := make([]byte, relayBufferSize)
	for {
		n, err := in.Read(buffer)
		if n > 0 {
			if writeErr := flush(buffer[:n]); writeErr != nil {
				return writeErr
			}
		}
		if err != nil {
			return err
		}
	}
}

func (t *serverTranscoder) cutText() error {
	header, err := t.read(7)
	if err != nil {
		return err
	}
	length, err := cutTextLength(header[3:])
	if err != nil {
		return err
	}
	body, err := t.read(length)
	if err != nil {
		return err
	}
	t.out.WriteByte(msgServerCutText)
	t.out.Write(header)
	t.out.Write(body)
	return nil
}

func (t *serverTranscoder) update() error {
	header, err := t.read(3)
	if err != nil {
		return err
	}
	t.out.WriteByte(msgFramebufferUpdate)
	t.out.Write(header)
	count := int(binary.BigEndian.Uint16(header[1:]))
	for i := 0; i < count || count == 0xffff; i++ {
		rect, err := t.read(12)
		if err != nil {
			return err
		}
		width := int(binary.BigEndian.Uint16(rect[4:]))
		height := int(binary.BigEndian.Uint16(rect[6:]))
		encoding := int32(binary.BigEndian.Uint32(rect[8:]))
		if err := t.rect(rect, width, height, encoding); err != nil {
			return err
		}
		if encoding == encodingLastRect {
			return nil
		}
		if t.out.Len() >= transcodeFlushSize {
			if err := t.send(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (t *serverTranscoder) rect(header []byte, width, height int, encoding int32) error {
	pixels := width * height
	switch encoding {
	case encodingRaw, encodingZlib, encodingZRLE, encodingCursor:
		if pixels > t.maxPixels {
			return fmt.Errorf("VNC rectangle of %dx%d is larger than the screen", width, height)
		}
	}
	switch encoding {
	case encodingRaw:
		t.out.Write(header)
		return t.convertPixels(pixels, t.read)
	case encodingCopyRect:
		body, err := t.read(4)
		t.out.Write(header)
		t.out.Write(body)
		return err
	case encodingZlib:
		data, err := t.rectData()
		if err != nil {
			return err
		}
		if err := t.zlibIn.feed(data); err != nil {
			return err
		}
		// Sent on as Raw: the loopback to the client has bandwidth to spare.
		raw := slices.Clone(header)
		binary.BigEndian.PutUint32(raw[8:], uint32(encodingRaw))
		t.out.Write(raw)
		return t.convertPixels(pixels, t.zlibIn.read)
	case encodingZRLE:
		t.out.Write(header)
		return t.zrle(width, height)
	case encodingCursor:
		if pixels > maxCursorPixels {
			return fmt.Errorf("VNC cursor of %dx%d is too large", width, height)
		}
		src, err := t.read(pixels * reducedPixelSize)
		if err != nil {
			return err
		}
		mask, err := t.read((width + 7) / 8 * height)
		if err != nil {
			return err
		}
		t.out.Write(header)
		t.out.Write(t.conv.pixels(nil, src))
		t.out.Write(mask)
		return nil
	case encodingDesktopSize:
		t.setFramebuffer(header[4:])
		t.out.Write(header)
		return nil
	case encodingLastRect:
		t.out.Write(header)
		return nil
	case encodingDesktopName:
		t.out.Write(header)
		return t.name()
	}
	return fmt.Errorf("unsupported VNC encoding %d", encoding)
}

// convertPixels converts pixel data a band at a time, so a large rectangle
// streams to the client and needs no buffer of its size.
func (t *serverTranscoder) convertPixels(pixels int, read func(int) ([]byte, error)) error {
	const chunk = 32 << 10
	buf := make([]byte, 0, chunk*4)
	for pixels > 0 {
		n := min(pixels, chunk)
		src, err := read(n * reducedPixelSize)
		if err != nil {
			return err
		}
		t.out.Write(t.conv.pixels(buf[:0], src))
		pixels -= n
		if t.out.Len() >= transcodeFlushSize {
			if err := t.send(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (t *serverTranscoder) rectData() ([]byte, error) {
	length, err := t.read(4)
	if err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(length)
	if n > maxRectData {
		return nil, fmt.Errorf("VNC rectangle of %d bytes is too large", n)
	}
	return t.read(int(n))
}

// zrle converts one ZRLE rectangle tile by tile into the client's compressed
// pixels, on a zlib stream of its own.
func (t *serverTranscoder) zrle(width, height int) error {
	data, err := t.rectData()
	if err != nil {
		return err
	}
	if err := t.zrleIn.feed(data); err != nil {
		return err
	}
	if t.zrleOut == nil {
		t.zrleOut, err = zlib.NewWriterLevel(&t.zrleData, zrleOutputLevel)
		if err != nil {
			return err
		}
	}
	var tile []byte
	for y := 0; y < height; y += zrleTileSize {
		for x := 0; x < width; x += zrleTileSize {
			w, h := min(zrleTileSize, width-x), min(zrleTileSize, height-y)
			tile, err = t.zrleTile(tile[:0], w, h)
			if err != nil {
				return err
			}
			if _, err := t.zrleOut.Write(tile); err != nil {
				return err
			}
		}
	}
	if err := t.zrleOut.Flush(); err != nil {
		return err
	}
	t.out.Write(binary.BigEndian.AppendUint32(nil, uint32(t.zrleData.Len())))
	t.out.Write(t.zrleData.Bytes())
	t.zrleData.Reset()
	return nil
}

// zrleTile appends one converted tile to dst.
func (t *serverTranscoder) zrleTile(dst []byte, w, h int) ([]byte, error) {
	in := &t.zrleIn
	sub, err := in.readByte()
	if err != nil {
		return nil, err
	}
	dst = append(dst, sub)
	cpixels := func(n int) error {
		src, err := in.read(n * reducedPixelSize)
		if err != nil {
			return err
		}
		dst = t.conv.cpixels(dst, src)
		return nil
	}
	switch {
	case sub == 0:
		err := cpixels(w * h)
		return dst, err
	case sub == 1:
		err := cpixels(1)
		return dst, err
	case sub >= 2 && sub <= 16:
		if err := cpixels(int(sub)); err != nil {
			return nil, err
		}
		bits := 4
		if sub == 2 {
			bits = 1
		} else if sub <= 4 {
			bits = 2
		}
		packed, err := in.read((w*bits + 7) / 8 * h)
		if err != nil {
			return nil, err
		}
		return append(dst, packed...), nil
	case sub == 128:
		for left := w * h; left > 0; {
			if err := cpixels(1); err != nil {
				return nil, err
			}
			run, err := t.copyRun(&dst)
			if err != nil {
				return nil, err
			}
			left -= run
		}
		return dst, nil
	case sub >= 130:
		if err := cpixels(int(sub) - 128); err != nil {
			return nil, err
		}
		for left := w * h; left > 0; {
			index, err := in.readByte()
			if err != nil {
				return nil, err
			}
			dst = append(dst, index)
			if index&128 == 0 {
				left--
				continue
			}
			run, err := t.copyRun(&dst)
			if err != nil {
				return nil, err
			}
			left -= run
		}
		return dst, nil
	}
	return nil, fmt.Errorf("invalid ZRLE subencoding %d", sub)
}

// copyRun copies a ZRLE run length to dst and returns the run it encodes.
func (t *serverTranscoder) copyRun(dst *[]byte) (int, error) {
	run := 1
	for {
		b, err := t.zrleIn.readByte()
		if err != nil {
			return 0, err
		}
		*dst = append(*dst, b)
		run += int(b)
		if b != 255 {
			return run, nil
		}
	}
}
