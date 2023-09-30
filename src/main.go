package main

import (
    "flag"
    "fmt"
    "image"
    "image/png"
    _ "image/gif"
    _ "image/jpeg"
    _ "image/png"
    "log"
    "os"
    "path/filepath"
    "strings"
    "time"
)

// Minimal CLI wrapper: parses flags, handles single/batch modes, and delegates to palette package.
func main() {
    var (
        inputFile   string
        colorCount  int
        jsonOutput  bool
        previewPath string
        inputDir    string
        outputDir   string
        stripWidth  int
    )

    flag.StringVar(&inputFile, "in", "", "input image path (png/jpg/gif)")
    flag.IntVar(&colorCount, "n", 8, "number of colors in the palette")
    flag.BoolVar(&jsonOutput, "json", false, "print palette as JSON")
    flag.StringVar(&previewPath, "preview", "", "path to save palette preview (PNG)")
    flag.StringVar(&inputDir, "IN", "", "input directory for batch processing")
    flag.StringVar(&outputDir, "out", "", "output directory for batch results")
    flag.IntVar(&stripWidth, "strip", 80, "palette strip width in pixels")
    flag.Parse()

    if colorCount <= 0 {
        log.Fatal("number of colors must be > 0")
    }

    // Batch mode: iterate files in inputDir, write composed PNGs to outputDir.
    if inputDir != "" && outputDir != "" {
        if err := os.MkdirAll(outputDir, 0o755); err != nil {
            log.Fatalf("cannot create output directory: %v", err)
        }
        entries, err := os.ReadDir(inputDir)
        if err != nil {
            log.Fatalf("cannot read input directory: %v", err)
        }
        for _, e := range entries {
            if e.IsDir() {
                continue
            }
            name := e.Name()
            if !isSupportedImage(name) {
                continue
            }
            inPath := filepath.Join(inputDir, name)
            outPath := filepath.Join(outputDir, replaceExt(name, ".png"))
            start := time.Now()
            log.Printf("%s: processing...", name)
            if err := processImage(inPath, outPath, colorCount, jsonOutput, previewPath, stripWidth); err != nil {
                log.Printf("%s: error: %v", name, err)
            } else {
                dur := time.Since(start)
                log.Printf("%s: done in %s", name, dur)
            }
        }
        return
    }

    if inputFile == "" {
        log.Fatal("provide input path via -in or use batch mode -IN/-out")
    }

    f, err := os.Open(inputFile)
    if err != nil {
        log.Fatalf("cannot open file: %v", err)
    }
    defer f.Close()

    img, _, err := image.Decode(f)
    if err != nil {
        log.Fatalf("cannot decode image: %v", err)
    }

    pixels := CollectPixels(img)
    palette := MedianCutPalette(pixels, colorCount)
    counts := CountOccurrences(pixels, palette)

    if jsonOutput {
        if err := PrintPaletteJSON(palette, counts); err != nil {
            log.Fatalf("JSON output error: %v", err)
        }
    } else {
        PrintPaletteText(palette, counts)
    }

    if previewPath != "" {
        if err := SavePalettePreview(previewPath, palette, counts); err != nil {
            log.Fatalf("failed to save preview: %v", err)
        }
        fmt.Printf("palette preview saved: %s\n", filepath.Clean(previewPath))
    }

    // If user wants composite output of single file, save into outputDir
    if outputDir != "" {
        if err := os.MkdirAll(outputDir, 0o755); err != nil {
            log.Fatalf("cannot create output directory: %v", err)
        }
        base := replaceExt(filepath.Base(inputFile), ".png")
        outPath := filepath.Join(outputDir, base)
        if err := saveComposite(outPath, img, palette, counts, stripWidth); err != nil {
            log.Fatalf("failed to save result: %v", err)
        }
    }
}

// processImage: read, decode, build palette, optional JSON/preview, then write composed image.
func processImage(inPath, outPath string, colors int, jsonOut bool, preview string, strip int) error {
    f, err := os.Open(inPath)
    if err != nil {
        return err
    }
    defer f.Close()
    img, _, err := image.Decode(f)
    if err != nil {
        return err
    }
    pixels := CollectPixels(img)
    palColors := MedianCutPalette(pixels, colors)
    counts := CountOccurrences(pixels, palColors)

    if jsonOut {
        if err := PrintPaletteJSON(palColors, counts); err != nil {
            return err
        }
    }
    if preview != "" {
        if err := SavePalettePreview(preview, palColors, counts); err != nil {
            return err
        }
    }
    return saveComposite(outPath, img, palColors, counts, strip)
}

// saveComposite writes PNG with the original content and palette strip appended on the right.
func saveComposite(path string, img image.Image, palette []RGB, counts []int, stripWidth int) error {
    composed := ComposeWithPaletteStrip(img, palette, counts, stripWidth)
    outFile, err := os.Create(path)
    if err != nil {
        return err
    }
    defer outFile.Close()
    return png.Encode(outFile, composed)
}

// isSupportedImage: basic extension check; decoder registration is done via blank imports above.
func isSupportedImage(name string) bool {
    ext := strings.ToLower(filepath.Ext(name))
    switch ext {
    case ".png", ".jpg", ".jpeg", ".gif":
        return true
    default:
        return false
    }
}

// replaceExt normalizes output filenames to PNG while preserving the base name.
func replaceExt(name, newExt string) string {
    base := strings.TrimSuffix(name, filepath.Ext(name))
    if !strings.HasPrefix(newExt, ".") {
        newExt = "." + newExt
    }
    return base + newExt
}

