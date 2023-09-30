package main

import (
    "encoding/json"
    "fmt"
    "image"
    "image/color"
    "image/png"
    "math"
    "os"
    "runtime"
    "sync"
    "sort"
)

type RGB struct {
    R uint8 `json:"r"`
    G uint8 `json:"g"`
    B uint8 `json:"b"`
}

// CollectPixels: fast-path for RGBA/NRGBA; fallback to generic At(). Avoids RGBA() per-pixel cost.
func CollectPixels(img image.Image) []RGB {
    b := img.Bounds()
    width, height := b.Dx(), b.Dy()
    n := width * height
    pixels := make([]RGB, n)

    switch src := img.(type) {
    case *image.RGBA:
        // 1) Fast path: tight loop over backing Pix for RGBA.
        i := 0
        for y := 0; y < height; y++ {
            row := src.Pix[y*src.Stride : y*src.Stride+width*4]
            for x := 0; x < width; x++ {
                off := x * 4
                pixels[i] = RGB{row[off], row[off+1], row[off+2]}
                i++
            }
        }
        return pixels
    case *image.NRGBA:
        // 2) Fast path: same idea for NRGBA.
        i := 0
        for y := 0; y < height; y++ {
            row := src.Pix[y*src.Stride : y*src.Stride+width*4]
            for x := 0; x < width; x++ {
                off := x * 4
                pixels[i] = RGB{row[off], row[off+1], row[off+2]}
                i++
            }
        }
        return pixels
    default:
        // 3) Generic path: use At()/RGBA() when memory layout is unknown.
        i := 0
        for y := b.Min.Y; y < b.Max.Y; y++ {
            for x := b.Min.X; x < b.Max.X; x++ {
                r, g, bb, _ := img.At(x, y).RGBA()
                pixels[i] = RGB{uint8(r >> 8), uint8(g >> 8), uint8(bb >> 8)}
                i++
            }
        }
        return pixels
    }
}

type colorBox struct {
    Pixels []RGB
}

func channelRange(pxs []RGB, ch int) int {
    if len(pxs) == 0 {
        return 0
    }
    minv, maxv := 255, 0
    for _, p := range pxs {
        var v int
        switch ch {
        case 0:
            v = int(p.R)
        case 1:
            v = int(p.G)
        case 2:
            v = int(p.B)
        }
        if v < minv {
            minv = v
        }
        if v > maxv {
            maxv = v
        }
    }
    return maxv - minv
}

// medianCutSplit: nth-element (quickselect) by dominant channel instead of full sort.
func medianCutSplit(pxs []RGB) ([]RGB, []RGB) {
    // 1) Pick dominant channel by range.
    ranges := []int{channelRange(pxs, 0), channelRange(pxs, 1), channelRange(pxs, 2)}
    dominant := 0
    if ranges[1] > ranges[dominant] {
        dominant = 1
    }
    if ranges[2] > ranges[dominant] {
        dominant = 2
    }
    // 2) Partition in-place around median along dominant channel.
    mid := len(pxs) / 2
    nthElementByChannel(pxs, mid, dominant)
    // 3) Split into two boxes.
    left := make([]RGB, mid)
    right := make([]RGB, len(pxs)-mid)
    copy(left, pxs[:mid])
    copy(right, pxs[mid:])
    return left, right
}

func nthElementByChannel(a []RGB, n int, ch int) {
    if n <= 0 || n >= len(a) {
        return
    }
    lo, hi := 0, len(a)-1
    for lo < hi {
        p := partitionByChannel(a, lo, hi, ch)
        if n == p {
            return
        } else if n < p {
            hi = p - 1
        } else {
            lo = p + 1
        }
    }
}

func channelValue(c RGB, ch int) uint8 {
    switch ch {
    case 0:
        return c.R
    case 1:
        return c.G
    default:
        return c.B
    }
}

func partitionByChannel(a []RGB, lo, hi, ch int) int {
    pivot := channelValue(a[(lo+hi)/2], ch)
    i, j := lo, hi
    for i <= j {
        for channelValue(a[i], ch) < pivot {
            i++
        }
        for channelValue(a[j], ch) > pivot {
            j--
        }
        if i <= j {
            a[i], a[j] = a[j], a[i]
            i++
            j--
        }
    }
    if lo <= j {
        if n := (lo + j) / 2; false { // no-op to keep structure similar to classic impl
            _ = n
        }
    }
    return i - 1
}

func averageColor(pxs []RGB) RGB {
    if len(pxs) == 0 {
        return RGB{0, 0, 0}
    }
    var rsum, gsum, bsum int64
    for _, p := range pxs {
        rsum += int64(p.R)
        gsum += int64(p.G)
        bsum += int64(p.B)
    }
    n := float64(len(pxs))
    r := uint8(math.Round(float64(rsum) / n))
    g := uint8(math.Round(float64(gsum) / n))
    b := uint8(math.Round(float64(bsum) / n))
    return RGB{r, g, b}
}

func medianColor(pxs []RGB) RGB {
    if len(pxs) == 0 {
        return RGB{0, 0, 0}
    }
    if len(pxs) <= 3 {
        var rsum, gsum, bsum int
        for _, p := range pxs {
            rsum += int(p.R)
            gsum += int(p.G)
            bsum += int(p.B)
        }
        n := len(pxs)
        return RGB{uint8(rsum / n), uint8(gsum / n), uint8(bsum / n)}
    }
    n := len(pxs)
    mid := n / 2
    if n%2 == 1 {
        return RGB{
            nthElementR(pxs, mid),
            nthElementG(pxs, mid),
            nthElementB(pxs, mid),
        }
    } else {
        r1, r2 := nthElementR(pxs, mid-1), nthElementR(pxs, mid)
        g1, g2 := nthElementG(pxs, mid-1), nthElementG(pxs, mid)
        b1, b2 := nthElementB(pxs, mid-1), nthElementB(pxs, mid)
        return RGB{
            uint8((int(r1) + int(r2)) / 2),
            uint8((int(g1) + int(g2)) / 2),
            uint8((int(b1) + int(b2)) / 2),
        }
    }
}

func nthElementR(pxs []RGB, k int) uint8 {
    temp := make([]uint8, len(pxs))
    for i, p := range pxs {
        temp[i] = p.R
    }
    return quickSelectUint8(temp, k)
}

func nthElementG(pxs []RGB, k int) uint8 {
    temp := make([]uint8, len(pxs))
    for i, p := range pxs {
        temp[i] = p.G
    }
    return quickSelectUint8(temp, k)
}

func nthElementB(pxs []RGB, k int) uint8 {
    temp := make([]uint8, len(pxs))
    for i, p := range pxs {
        temp[i] = p.B
    }
    return quickSelectUint8(temp, k)
}

func quickSelectUint8(arr []uint8, k int) uint8 {
    if k >= len(arr) {
        k = len(arr) - 1
    }
    lo, hi := 0, len(arr)-1
    for lo < hi {
        p := partitionUint8(arr, lo, hi)
        if k == p {
            return arr[k]
        } else if k < p {
            hi = p - 1
        } else {
            lo = p + 1
        }
    }
    return arr[lo]
}

func partitionUint8(arr []uint8, lo, hi int) int {
    pivot := arr[(lo+hi)/2]
    i, j := lo, hi
    for i <= j {
        for arr[i] < pivot {
            i++
        }
        for arr[j] > pivot {
            j--
        }
        if i <= j {
            arr[i], arr[j] = arr[j], arr[i]
            i++
            j--
        }
    }
    return i - 1
}

func MedianCutPalette(pixels []RGB, k int) []RGB {
    if k <= 0 {
        return nil
    }
    // 1) Trivial cases.
    if k == 1 {
        return []RGB{averageColor(pixels)}
    }
    if len(pixels) <= k {
        result := make([]RGB, len(pixels))
        copy(result, pixels)
        for len(result) < k {
            result = append(result, result[len(result)-1])
        }
        return result
    }
    // 2) Start from a single box and iteratively split the widest.
    boxes := make([]colorBox, 1, k)
    boxes[0] = colorBox{Pixels: pixels}
    
    for len(boxes) < k {
        // 2.1) Find the box with max channel spread.
        widestIdx := -1
        widestRange := -1
        for i := range boxes {
            if len(boxes[i].Pixels) <= 1 {
                continue
            }
            r := channelRange(boxes[i].Pixels, 0)
            g := channelRange(boxes[i].Pixels, 1)
            bRange := channelRange(boxes[i].Pixels, 2)
            maxRange := r
            if g > maxRange {
                maxRange = g
            }
            if bRange > maxRange {
                maxRange = bRange
            }
            if maxRange > widestRange {
                widestRange = maxRange
                widestIdx = i
            }
        }
        if widestIdx == -1 {
            break
        }
        // 2.2) Split by median cut along dominant channel.
        left, right := medianCutSplit(boxes[widestIdx].Pixels)
        // 2.3) Replace original with left, append right.
        boxes[widestIdx] = colorBox{Pixels: left}
        boxes = append(boxes, colorBox{Pixels: right})
    }

    // 3) Reduce each box to a representative color (median per channel).
    palette := make([]RGB, 0, len(boxes))
    for i := range boxes {
        b := &boxes[i]
        palette = append(palette, medianColor(b.Pixels))
    }
    // 4) Pad if splits ran out early.
    for len(palette) < k {
        palette = append(palette, palette[len(palette)-1])
    }
    return palette
}

// CountOccurrences: single-thread for small inputs; fan-out with goroutines for large.
func CountOccurrences(pixels []RGB, palette []RGB) []int {
    if len(palette) == 0 || len(pixels) == 0 {
        return make([]int, len(palette))
    }
    // 1) Small inputs: single-thread; large: fan-out by chunks.
    workers := runtime.GOMAXPROCS(0)
    if workers < 2 || len(pixels) < 5000 {
        
        counts := make([]int, len(palette))
        for _, px := range pixels {
            bestIdx := 0
            best := colorDistanceSqInt(px, palette[0])
            for i := 1; i < len(palette); i++ {
                d := colorDistanceSqInt(px, palette[i])
                if d < best {
                    best = d
                    bestIdx = i
                }
            }
            counts[bestIdx]++
        }
        return counts
    }
    // 2) Split into roughly equal parts and process in parallel.
    type part struct{ from, to int }
    parts := make([]part, 0, workers)
    step := (len(pixels) + workers - 1) / workers
    for i := 0; i < len(pixels); i += step {
        j := i + step
        if j > len(pixels) {
            j = len(pixels)
        }
        parts = append(parts, part{from: i, to: j})
    }
    partials := make([][]int, len(parts))
    var wg sync.WaitGroup
    wg.Add(len(parts))
    for idx, pr := range parts {
        idx, pr := idx, pr
        go func() {
            defer wg.Done()
            cnt := make([]int, len(palette))
            for _, px := range pixels[pr.from:pr.to] {
                bestIdx := 0
                best := colorDistanceSqInt(px, palette[0])
                for i := 1; i < len(palette); i++ {
                    d := colorDistanceSqInt(px, palette[i])
                    if d < best {
                        best = d
                        bestIdx = i
                    }
                }
                cnt[bestIdx]++
            }
            partials[idx] = cnt
        }()
    }
    wg.Wait()
    // 3) Merge partial histograms.
    counts := make([]int, len(palette))
    for _, p := range partials {
        for i := range counts {
            counts[i] += p[i]
        }
    }
    return counts
}

// colorDistanceSqInt: int math to avoid float overhead.
func colorDistanceSqInt(a, b RGB) int {
    dr := int(a.R) - int(b.R)
    dg := int(a.G) - int(b.G)
    db := int(a.B) - int(b.B)
    return dr*dr + dg*dg + db*db
}

type PaletteEntry struct {
    Color  RGB `json:"color"`
    Count  int `json:"count"`
    Share  float64 `json:"share"`
    Hex    string `json:"hex"`
}

func PrintPaletteText(palette []RGB, counts []int) {
    entries := makeEntries(palette, counts)
    for _, e := range entries {
        fmt.Printf("%s\tcount=%d\tshare=%.2f%%\n", e.Hex, e.Count, e.Share*100)
    }
}

func PrintPaletteJSON(palette []RGB, counts []int) error {
    entries := makeEntries(palette, counts)
    enc := json.NewEncoder(os.Stdout)
    enc.SetIndent("", "  ")
    return enc.Encode(entries)
}

func makeEntries(palette []RGB, counts []int) []PaletteEntry {
    total := 0
    for _, c := range counts {
        total += c
    }
    entries := make([]PaletteEntry, 0, len(palette))
    for i, c := range palette {
        share := 0.0
        if total > 0 {
            share = float64(counts[i]) / float64(total)
        }
        entries = append(entries, PaletteEntry{
            Color: c,
            Count: counts[i],
            Share: share,
            Hex:   toHex(c),
        })
    }
    sort.Slice(entries, func(i, j int) bool { return entries[i].Count > entries[j].Count })
    return entries
}

func toHex(c RGB) string {
    return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
}

func SavePalettePreview(path string, palette []RGB, counts []int) error {
    entries := makeEntries(palette, counts)
    const width = 600
    const height = 60
    img := image.NewRGBA(image.Rect(0, 0, width, height))
    total := 0
    for _, e := range entries {
        total += e.Count
    }
    if total == 0 {
        total = 1
    }
    x := 0
    for _, e := range entries {
        w := int(math.Round(float64(width) * float64(e.Count) / float64(total)))
        if w <= 0 {
            continue
        }
        fill := color.RGBA{R: e.Color.R, G: e.Color.G, B: e.Color.B, A: 255}
        for yi := 0; yi < height; yi++ {
            for xi := x; xi < x+w && xi < width; xi++ {
                img.SetRGBA(xi, yi, fill)
            }
        }
        x += w
    }

    f, err := os.Create(path)
    if err != nil {
        return err
    }
    defer f.Close()
    return png.Encode(f, img)
}

// ComposeWithPaletteStrip returns a new image: original content with a vertical palette strip on the right.
// The strip shows colors sorted by frequency, stacked vertically with heights proportional to shares.
func ComposeWithPaletteStrip(src image.Image, palette []RGB, counts []int, stripWidth int) image.Image {
    if stripWidth <= 0 {
        stripWidth = 1
    }
    entries := makeEntries(palette, counts)
    b := src.Bounds()
    w := b.Dx()
    h := b.Dy()
    out := image.NewRGBA(image.Rect(0, 0, w+stripWidth, h))

    // 1) Copy source pixels (fast path for RGBA; fallback to At()).
    if rgba, ok := src.(*image.RGBA); ok && b.Min.X == 0 && b.Min.Y == 0 {
        for y := 0; y < h; y++ {
            srcRow := rgba.Pix[y*rgba.Stride : y*rgba.Stride+w*4]
            dstRow := out.Pix[y*out.Stride : y*out.Stride+w*4]
            copy(dstRow, srcRow)
        }
    } else {
        for y := 0; y < h; y++ {
            for x := 0; x < w; x++ {
                out.Set(x, y, src.At(b.Min.X+x, b.Min.Y+y))
            }
        }
    }

    // 2) Draw vertical palette strip with heights proportional to shares.
    total := 0
    for _, e := range entries {
        total += e.Count
    }
    if total == 0 {
        total = 1
    }
    yCursor := 0
    for i, e := range entries {
        blockH := int(math.Round(float64(h) * e.Share))
        if e.Count > 0 && blockH == 0 {
            blockH = 1
        }
        if i == len(entries)-1 {
            if yCursor+blockH < h {
                blockH = h - yCursor
            }
        }
        for yy := yCursor; yy < yCursor+blockH && yy < h; yy++ {
            rowStart := yy * out.Stride
            for xx := w; xx < w+stripWidth; xx++ {
                idx := rowStart + xx*4
                out.Pix[idx] = e.Color.R
                out.Pix[idx+1] = e.Color.G
                out.Pix[idx+2] = e.Color.B
                out.Pix[idx+3] = 255
            }
        }
        yCursor += blockH
        if yCursor >= h {
            break
        }
    }
    // 3) Fill any rounding gap with the last color.
    if yCursor < h && len(entries) > 0 {
        last := entries[len(entries)-1].Color
        for yy := yCursor; yy < h; yy++ {
            rowStart := yy * out.Stride
            for xx := w; xx < w+stripWidth; xx++ {
                idx := rowStart + xx*4
                out.Pix[idx] = last.R
                out.Pix[idx+1] = last.G
                out.Pix[idx+2] = last.B
                out.Pix[idx+3] = 255
            }
        }
    }
    return out
}

