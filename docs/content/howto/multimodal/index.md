+++
title = 'Sending Images, Audio, Video, and Documents to LLMs'
date = '2026-06-03'
draft = false
weight = 2
+++

# Sending Binary Files to LLMs

bunshin-go supports multi-modal content through `ContentPart`. Any binary file — image, audio clip, video, PDF — is wrapped in a `ContentPart` with `Data []byte` and `MimeType string`. Provider adapters base64-encode the bytes at the wire layer; your code works with raw `[]byte` throughout.

---

## Provider support matrix

| Content type | OpenAI | Anthropic | Google Gemini | Azure OpenAI |
|-------------|--------|-----------|---------------|--------------|
| Image (URL) | ✓ | ✓ | ✓ | ✓ |
| Image (inline bytes) | ✓ jpg/png/gif/webp | ✓ jpg/png/gif/webp | ✓ jpg/png/gif/webp | ✓ |
| Audio | ✓ wav/mp3 | ✗ | ✓ wav/mp3/ogg/flac/aac | ✗ |
| Video | ✗ | ✗ | ✓ mp4/mpeg/mov/avi/webm | ✗ |
| Document (PDF) | ✗ | ✓ pdf | ✓ pdf/plain | ✗ |

---

## Images

### Image from URL

```go
msg := llm.Message{
    Role: llm.RoleUser,
    Parts: []llm.ContentPart{
        {Type: llm.PartTypeText, Text: "What is in this image?"},
        {Type: llm.PartTypeImageURL, ImageURL: "https://example.com/photo.jpg"},
    },
}
resp, err := provider.Complete(ctx, &llm.Request{Messages: []llm.Message{msg}})
```

### Image from disk (inline bytes)

```go
data, err := os.ReadFile("photo.jpg")
if err != nil {
    return err
}

msg := llm.Message{
    Role: llm.RoleUser,
    Parts: []llm.ContentPart{
        {Type: llm.PartTypeText, Text: "Describe this image in detail."},
        {
            Type:     llm.PartTypeImageData,
            Data:     data,
            MimeType: "image/jpeg",
        },
    },
}
resp, err := provider.Complete(ctx, &llm.Request{Messages: []llm.Message{msg}})
```

### Multiple images in one message

```go
img1, _ := os.ReadFile("before.png")
img2, _ := os.ReadFile("after.png")

msg := llm.Message{
    Role: llm.RoleUser,
    Parts: []llm.ContentPart{
        {Type: llm.PartTypeText, Text: "Compare these two screenshots and list the differences."},
        {Type: llm.PartTypeImageData, Data: img1, MimeType: "image/png"},
        {Type: llm.PartTypeImageData, Data: img2, MimeType: "image/png"},
    },
}
```

---

## Audio

Audio analysis is supported by OpenAI and Google Gemini.

```go
audio, err := os.ReadFile("meeting.wav")
if err != nil {
    return err
}

// Google Gemini supports audio transcription and analysis
googleProvider := llm.NewGoogleProvider(llm.GoogleConfig{
    APIKey: key,
    Model:  "gemini-2.0-flash",
})

resp, err := googleProvider.Complete(ctx, &llm.Request{
    Messages: []llm.Message{{
        Role: llm.RoleUser,
        Parts: []llm.ContentPart{
            {Type: llm.PartTypeText, Text: "Transcribe this audio and summarise the key discussion points."},
            {
                Type:     llm.PartTypeAudio,
                Data:     audio,
                MimeType: "audio/wav",
            },
        },
    }},
})
fmt.Println(resp.Content)
```

Supported MIME types:
- `audio/wav` — WAV
- `audio/mpeg` — MP3
- `audio/ogg` — OGG (Gemini only)
- `audio/flac` — FLAC (Gemini only)
- `audio/aac` — AAC (Gemini only)

---

## Video

Video analysis is supported by Google Gemini.

```go
video, err := os.ReadFile("demo.mp4")
if err != nil {
    return err
}

resp, err := googleProvider.Complete(ctx, &llm.Request{
    Messages: []llm.Message{{
        Role: llm.RoleUser,
        Parts: []llm.ContentPart{
            {Type: llm.PartTypeText, Text: "Describe what happens in this video. What actions are performed?"},
            {
                Type:     llm.PartTypeVideo,
                Data:     video,
                MimeType: "video/mp4",
            },
        },
    }},
})
```

Supported MIME types:
- `video/mp4`
- `video/mpeg`
- `video/quicktime` — MOV
- `video/x-msvideo` — AVI
- `video/webm`

**Note**: Large video files should be uploaded via the Gemini Files API rather than inline bytes. For files over 20 MB, use the provider's file upload API and reference the file URI in a `PartTypeImageURL` part.

---

## Documents (PDF)

PDFs are supported by Anthropic and Google Gemini.

```go
pdf, err := os.ReadFile("contract.pdf")
if err != nil {
    return err
}

// Anthropic Claude handles PDFs well
anthropicProvider := llm.NewAnthropicProvider(llm.AnthropicConfig{
    APIKey: key,
    Model:  "claude-3-5-sonnet-20241022",
})

resp, err := anthropicProvider.Complete(ctx, &llm.Request{
    Messages: []llm.Message{{
        Role: llm.RoleUser,
        Parts: []llm.ContentPart{
            {
                Type:     llm.PartTypeDocument,
                Data:     pdf,
                MimeType: "application/pdf",
            },
            {Type: llm.PartTypeText, Text: "Extract all payment terms and deadlines from this contract."},
        },
    }},
})
```

---

## Multi-modal in a Runnable

Wrap multi-modal analysis in a `Runnable` so it composes into chains and graphs:

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

    // Detect MIME type from extension
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
                {Type: llm.PartTypeText, Text: req.Question},
                {Type: llm.PartTypeImageData, Data: data, MimeType: mimeType},
            },
        }},
    })
    if err != nil {
        return nil, err
    }
    return resp.Content, nil
})

// Use in a chain: extract text from image, then reason over the text
pipeline := chain.New("ocr-then-reason",
    chain.Step{ID: "ocr", Runnable: analyseImage},
    chain.Step{ID: "reason", Runnable: reasonRunnable},
)

result, err := pipeline.Invoke(ctx, ImageAnalysisInput{
    ImagePath: "invoice.png",
    Question:  "Extract all line items and their amounts as JSON.",
})
```

---

## Helper: read file as ContentPart

```go
// FileContentPart reads a file from disk and returns a ContentPart with the correct type.
func FileContentPart(path string) (llm.ContentPart, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return llm.ContentPart{}, err
    }

    ext := strings.ToLower(filepath.Ext(path))
    mimeTypes := map[string]struct {
        mime     string
        partType llm.ContentPartType
    }{
        ".jpg":  {"image/jpeg", llm.PartTypeImageData},
        ".jpeg": {"image/jpeg", llm.PartTypeImageData},
        ".png":  {"image/png", llm.PartTypeImageData},
        ".gif":  {"image/gif", llm.PartTypeImageData},
        ".webp": {"image/webp", llm.PartTypeImageData},
        ".wav":  {"audio/wav", llm.PartTypeAudio},
        ".mp3":  {"audio/mpeg", llm.PartTypeAudio},
        ".ogg":  {"audio/ogg", llm.PartTypeAudio},
        ".flac": {"audio/flac", llm.PartTypeAudio},
        ".mp4":  {"video/mp4", llm.PartTypeVideo},
        ".mov":  {"video/quicktime", llm.PartTypeVideo},
        ".webm": {"video/webm", llm.PartTypeVideo},
        ".pdf":  {"application/pdf", llm.PartTypeDocument},
    }

    info, ok := mimeTypes[ext]
    if !ok {
        return llm.ContentPart{}, fmt.Errorf("unsupported file type: %s", ext)
    }

    return llm.ContentPart{
        Type:     info.partType,
        Data:     data,
        MimeType: info.mime,
    }, nil
}
```

Usage:

```go
part, err := FileContentPart("chart.png")
if err != nil {
    return err
}

msg := llm.Message{
    Role: llm.RoleUser,
    Parts: []llm.ContentPart{
        {Type: llm.PartTypeText, Text: "Explain this chart."},
        part,
    },
}
```
