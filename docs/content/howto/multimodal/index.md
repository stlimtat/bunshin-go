+++
title = 'Sending Images, Audio, Video, and Documents to LLMs'
date = '2026-06-03'
draft = false
toc = true
weight = 2
+++

# Sending Binary Files to LLMs

bunshin-go models multi-modal content through `ContentPart`. Each part carries a `Type` discriminant, and binary content uses an `io.Reader` + `DataSize int64` pair so large files are streamed rather than copied into memory.

> **Implementation status**
>
> `PartTypeImage` is the only part type currently wired through the provider adapters. Whether the image is sent by URL or inline bytes is determined by the `Media.URL` or `Media.Data` field on the `MediaRef`, not by separate type constants. Audio, video, and document types (`PartTypeAudio`, `PartTypeVideo`, `PartTypeDocument`) are defined in the API and validated, but no adapter converts them to wire format yet. Use the constructor functions described below — they will work transparently once adapter support lands.

---

## Provider support matrix

URL vs inline delivery is selected automatically from `Media.URL` (set by `NewImageURLPart`) vs `Media.Data` (set by `NewBinaryPart`/`NewBinaryPartFromBytes`) — both use `PartTypeImage`.

| Content type | OpenAI | Anthropic | Google Gemini | Azure OpenAI |
|-------------|--------|-----------|---------------|--------------|
| Image (`PartTypeImage`, URL via `Media.URL`) | ✓ | ✓ | ✓ | ✓ |
| Image (`PartTypeImage`, inline via `Media.Data`) | ✓ jpg/png/gif/webp | ✓ jpg/png/gif/webp | ✓ jpg/png/gif/webp | ✓ |
| Audio (`PartTypeAudio`) | planned | — | planned | — |
| Video (`PartTypeVideo`) | — | — | planned | — |
| Document (`PartTypeDocument`, PDF) | — | planned | planned | — |

---

## Constructor functions

Prefer the constructor functions over struct literals — they set all required fields correctly and are forward-compatible with API changes.

| Function | Use for | Sets |
|---------|---------|------|
| `llm.NewTextPart(text)` | Plain text | — |
| `llm.NewImageURLPart(url)` | Remote image by URL | `Media.URL` |
| `llm.NewBinaryPartFromBytes(partType, data, mimeType)` | Small in-memory buffers | `Media.Data`, `Media.MimeType` |
| `llm.NewBinaryPart(partType, reader, size, mimeType)` | Large files via `io.Reader` | `Media.Data`, `Media.MimeType` |

---

## Images

### Image from URL

```go
msg := llm.Message{
    Role: llm.RoleUser,
    Parts: []llm.ContentPart{
        llm.NewTextPart("What is in this image?"),
        llm.NewImageURLPart("https://example.com/photo.jpg"),
    },
}
resp, err := provider.Complete(ctx, &llm.Request{Messages: []llm.Message{msg}})
```

### Image from disk (small file, inline bytes)

```go
data, err := os.ReadFile("photo.jpg")
if err != nil {
    return err
}

msg := llm.Message{
    Role: llm.RoleUser,
    Parts: []llm.ContentPart{
        llm.NewTextPart("Describe this image in detail."),
        llm.NewBinaryPartFromBytes(llm.PartTypeImage, data, "image/jpeg"),
    },
}
resp, err := provider.Complete(ctx, &llm.Request{Messages: []llm.Message{msg}})
```

### Image from disk (large file, streamed)

```go
f, err := os.Open("photo.jpg")
if err != nil {
    return err
}
defer f.Close()

info, _ := f.Stat()

msg := llm.Message{
    Role: llm.RoleUser,
    Parts: []llm.ContentPart{
        llm.NewTextPart("Describe this image in detail."),
        llm.NewBinaryPart(llm.PartTypeImage, f, info.Size(), "image/jpeg"),
    },
}
```

### Multiple images in one message

```go
img1, _ := os.ReadFile("before.png")
img2, _ := os.ReadFile("after.png")

msg := llm.Message{
    Role: llm.RoleUser,
    Parts: []llm.ContentPart{
        llm.NewTextPart("Compare these two screenshots and list the differences."),
        llm.NewBinaryPartFromBytes(llm.PartTypeImage, img1, "image/png"),
        llm.NewBinaryPartFromBytes(llm.PartTypeImage, img2, "image/png"),
    },
}
```

---

## Audio

> **Planned support** — `PartTypeAudio` is defined but no provider adapter sends it to the wire yet. The code below shows the intended API.

```go
audio, err := os.ReadFile("meeting.wav")
if err != nil {
    return err
}

resp, err := googleProvider.Complete(ctx, &llm.Request{
    Messages: []llm.Message{{
        Role: llm.RoleUser,
        Parts: []llm.ContentPart{
            llm.NewTextPart("Transcribe this audio and summarise the key discussion points."),
            llm.NewBinaryPartFromBytes(llm.PartTypeAudio, audio, "audio/wav"),
        },
    }},
})
```

Supported MIME types (once wired):
- `audio/wav` — WAV
- `audio/mpeg` — MP3
- `audio/ogg` — OGG (Gemini)
- `audio/flac` — FLAC (Gemini)
- `audio/aac` — AAC (Gemini)

---

## Video

> **Planned support** — `PartTypeVideo` is defined but no provider adapter sends it to the wire yet.

```go
f, err := os.Open("demo.mp4")
if err != nil {
    return err
}
defer f.Close()
info, _ := f.Stat()

resp, err := googleProvider.Complete(ctx, &llm.Request{
    Messages: []llm.Message{{
        Role: llm.RoleUser,
        Parts: []llm.ContentPart{
            llm.NewTextPart("Describe what happens in this video."),
            llm.NewBinaryPart(llm.PartTypeVideo, f, info.Size(), "video/mp4"),
        },
    }},
})
```

Supported MIME types (once wired):
- `video/mp4`
- `video/mpeg`
- `video/quicktime` — MOV
- `video/x-msvideo` — AVI
- `video/webm`

For files over 20 MB, prefer the provider's file upload API and reference the URI via `NewImageURLPart`.

---

## Documents (PDF)

> **Planned support** — `PartTypeDocument` is defined but no provider adapter sends it to the wire yet.

```go
pdf, err := os.ReadFile("contract.pdf")
if err != nil {
    return err
}

resp, err := anthropicProvider.Complete(ctx, &llm.Request{
    Messages: []llm.Message{{
        Role: llm.RoleUser,
        Parts: []llm.ContentPart{
            llm.NewBinaryPartFromBytes(llm.PartTypeDocument, pdf, "application/pdf"),
            llm.NewTextPart("Extract all payment terms and deadlines from this contract."),
        },
    }},
})
```

---

## Multi-modal in a Runnable

Wrap multi-modal analysis in a `Runnable` to compose into chains and graphs:

```go
type ImageAnalysisInput struct {
    ImagePath string
    Question  string
}

analyseImage := core.NewRunnableFunc("analyse-image", func(ctx context.Context, input any) (any, error) {
    req := input.(ImageAnalysisInput)

    data, err := os.ReadFile(req.ImagePath)
    if err != nil {
        return nil, fmt.Errorf("read image: %w", err)
    }

    mimeType := "image/jpeg"
    switch strings.ToLower(filepath.Ext(req.ImagePath)) {
    case ".png":
        mimeType = "image/png"
    case ".gif":
        mimeType = "image/gif"
    case ".webp":
        mimeType = "image/webp"
    }

    resp, err := visionProvider.Complete(ctx, &llm.Request{
        Messages: []llm.Message{{
            Role: llm.RoleUser,
            Parts: []llm.ContentPart{
                llm.NewTextPart(req.Question),
                llm.NewBinaryPartFromBytes(llm.PartTypeImage, data, mimeType),
            },
        }},
    })
    if err != nil {
        return nil, err
    }
    return resp.Content, nil
})

// Extract text from image, then reason over the text.
// Define a shared State struct (e.g. OcrState) when both steps read/write structured fields.
pipeline := chain.New[any]("ocr-then-reason",
    chain.Step[any]{ID: "ocr", Runnable: analyseImage},
    chain.Step[any]{ID: "reason", Runnable: reasonRunnable},
)

result, err := pipeline.Invoke(ctx, ImageAnalysisInput{
    ImagePath: "invoice.png",
    Question:  "Extract all line items and their amounts as JSON.",
})
```

---

## Helper: detect content type from file extension

```go
// FileContentPart opens a file and returns a ContentPart with the correct type.
// The caller is responsible for closing the file after the request completes.
func FileContentPart(path string) (llm.ContentPart, *os.File, error) {
    f, err := os.Open(path)
    if err != nil {
        return llm.ContentPart{}, nil, err
    }
    info, err := f.Stat()
    if err != nil {
        f.Close()
        return llm.ContentPart{}, nil, err
    }

    type typeInfo struct {
        mime     string
        partType llm.ContentPartType
    }
    ext := strings.ToLower(filepath.Ext(path))
    types := map[string]typeInfo{
        ".jpg":  {"image/jpeg", llm.PartTypeImage},
        ".jpeg": {"image/jpeg", llm.PartTypeImage},
        ".png":  {"image/png", llm.PartTypeImage},
        ".gif":  {"image/gif", llm.PartTypeImage},
        ".webp": {"image/webp", llm.PartTypeImage},
        ".wav":  {"audio/wav", llm.PartTypeAudio},
        ".mp3":  {"audio/mpeg", llm.PartTypeAudio},
        ".ogg":  {"audio/ogg", llm.PartTypeAudio},
        ".flac": {"audio/flac", llm.PartTypeAudio},
        ".mp4":  {"video/mp4", llm.PartTypeVideo},
        ".mov":  {"video/quicktime", llm.PartTypeVideo},
        ".webm": {"video/webm", llm.PartTypeVideo},
        ".pdf":  {"application/pdf", llm.PartTypeDocument},
    }

    t, ok := types[ext]
    if !ok {
        f.Close()
        return llm.ContentPart{}, nil, fmt.Errorf("unsupported file type: %s", ext)
    }

    return llm.NewBinaryPart(t.partType, f, info.Size(), t.mime), f, nil
}
```

Usage:

```go
part, f, err := FileContentPart("chart.png")
if err != nil {
    return err
}
defer f.Close()

msg := llm.Message{
    Role: llm.RoleUser,
    Parts: []llm.ContentPart{
        llm.NewTextPart("Explain this chart."),
        part,
    },
}
```
