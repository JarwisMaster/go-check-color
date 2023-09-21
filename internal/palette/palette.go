package palette

import (
    "encoding/json"
    "fmt"
    "image"
    "image/color"
    "image/png"
    "math"
    "os"
    "sort"
)

type RGB struct {
    R uint8 `json:"r"`
    G uint8 `json:"g"`
    B uint8 `json:"b"`
}

func CollectPixels(img image.Image) []RGB {
    b := img.Bounds()
    width, height := b.Dx(), b.Dy()
    pixels := make([]RGB, 0, width*height)
    for y := b.Min.Y; y < b.Max.Y; y++ {
        for x := b.Min.X; x < b.Max.X; x++ {
            r, g, b, _ := img.At(x, y).RGBA()
            pixels = append(pixels, RGB{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)})
        }
    }
    return pixels
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

func medianCutSplit(pxs []RGB) ([]RGB, []RGB) {
    ranges := []int{channelRange(pxs, 0), channelRange(pxs, 1), channelRange(pxs, 2)}
    dominant := 0
    if ranges[1] > ranges[dominant] {
        dominant = 1
    }
    if ranges[2] > ranges[dominant] {
        dominant = 2
    }

    sort.Slice(pxs, func(i, j int) bool {
        switch dominant {
        case 0:
            return pxs[i].R < pxs[j].R
        case 1:
            return pxs[i].G < pxs[j].G
        default:
            return pxs[i].B < pxs[j].B
        }
    })

    mid := len(pxs) / 2
    left := make([]RGB, mid)
    right := make([]RGB, len(pxs)-mid)
    copy(left, pxs[:mid])
    copy(right, pxs[mid:])
    return left, right
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
    n := len(pxs)
    rr := make([]int, n)
    gg := make([]int, n)
    bb := make([]int, n)
    for i, p := range pxs {
        rr[i] = int(p.R)
        gg[i] = int(p.G)
        bb[i] = int(p.B)
    }
    sort.Ints(rr)
    sort.Ints(gg)
    sort.Ints(bb)
    if n%2 == 1 {
        mid := n / 2
        return RGB{uint8(rr[mid]), uint8(gg[mid]), uint8(bb[mid])}
    }
    r := uint8(math.Round(float64(rr[n/2-1]+rr[n/2]) / 2.0))
    g := uint8(math.Round(float64(gg[n/2-1]+gg[n/2]) / 2.0))
    b := uint8(math.Round(float64(bb[n/2-1]+bb[n/2]) / 2.0))
    return RGB{r, g, b}
}

func MedianCutPalette(pixels []RGB, k int) []RGB {
    if k <= 0 {
        return nil
    }
    boxes := []colorBox{{Pixels: pixels}}
    for len(boxes) < k {
        widestIdx := 0
        widestRange := -1
        for i, b := range boxes {
            r := channelRange(b.Pixels, 0)
            g := channelRange(b.Pixels, 1)
            bRange := channelRange(b.Pixels, 2)
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

        if len(boxes[widestIdx].Pixels) <= 1 {
            break
        }

        left, right := medianCutSplit(boxes[widestIdx].Pixels)
        boxes = append(boxes[:widestIdx], append([]colorBox{{Pixels: left}, {Pixels: right}}, boxes[widestIdx+1:]...)...)
    }

    palette := make([]RGB, 0, len(boxes))
    for _, b := range boxes {
        palette = append(palette, medianColor(b.Pixels))
    }
    for len(palette) < k && len(palette) > 0 {
        palette = append(palette, palette[len(palette)-1])
    }
    return palette
}

func CountOccurrences(pixels []RGB, palette []RGB) []int {
    counts := make([]int, len(palette))
    for _, p := range pixels {
        bestIdx := 0
        bestDist := math.MaxFloat64
        for i, c := range palette {
            d := colorDistanceSq(p, c)
            if d < bestDist {
                bestDist = d
                bestIdx = i
            }
        }
        counts[bestIdx]++
    }
    return counts
}

func colorDistanceSq(a, b RGB) float64 {
    dr := float64(int(a.R) - int(b.R))
    dg := float64(int(a.G) - int(b.G))
    db := float64(int(a.B) - int(b.B))
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

