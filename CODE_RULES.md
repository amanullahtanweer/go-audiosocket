# AudioSocket Transcriber - Code Rules & Best Practices

## 🎯 CRITICAL AUDIO RULES

### **AudioSocket Audio Playback - NEVER FORGET!**

#### **Chunk Size Rules:**
- **ALWAYS use `audiosocket.DefaultSlinChunkSize = 320 bytes`**
- **NEVER use custom chunk sizes like 160 bytes**
- **320 bytes = 8000Hz × 20ms × 2 bytes** (correct calculation)
- **160 bytes = 8000Hz × 10ms × 2 bytes** (WRONG - causes slow motion!)

#### **Audio Playback Implementation:**
```go
// ✅ CORRECT - Use built-in function
if err := audiosocket.SendSlinChunks(conn, audiosocket.DefaultSlinChunkSize, audioData); err != nil {
    return fmt.Errorf("failed to send audio: %w", err)
}

// ❌ WRONG - Custom ticker and wrong chunk size
chunkSize := 160  // WRONG!
ticker := time.NewTicker(20 * time.Millisecond)  // WRONG!
```

#### **First Chunk Alignment Rule - CRITICAL!**
```go
// ✅ CORRECT - Ensure first chunk is perfectly aligned
chunkSize := audiosocket.DefaultSlinChunkSize
startOffset := 0
if len(audioData) > chunkSize && len(audioData)%chunkSize != 0 {
    // Find first complete chunk boundary to avoid 0.1s distortion
    startOffset = chunkSize - (len(audioData) % chunkSize)
    if startOffset >= len(audioData) {
        startOffset = 0
    }
}
// Start from offset to ensure clean first chunk
for i := startOffset; i < len(audioData); i += chunkSize {
    // ... chunk processing
}
```

#### **Why This Matters:**
- **Wrong chunk size = Audio plays in slow motion**
- **Custom timing = Audio corruption and distortion**
- **Built-in function = Proper AudioSocket protocol implementation**

---

## 🔧 PROJECT ARCHITECTURE RULES

### **File Organization:**
```
cmd/server/main.go          # Main entry point, config loading
internal/server/server.go    # Server logic, session management
internal/audio/player.go     # Audio playback functionality
internal/transcriber/        # Transcription providers
```

### **Configuration Rules:**
- **Always validate transcription provider** (vosk/assemblyai)
- **Create output directories** if they don't exist
- **Set default audio directory** to `./audios`

### **Audio File Rules:**
- **WAV format**: 8kHz, 16-bit PCM, Mono
- **Directory**: `./audios/` for main files, `./audios/background/` for ambient
- **Pre-load all audio files** at startup for performance

---

## 🚫 COMMON MISTAKES TO AVOID

### **Audio Playback:**
1. **Don't hardcode WAV header size** (44 bytes) - parse properly
2. **Don't use custom tickers** - use `audiosocket.SendSlinChunks()`
3. **Don't guess chunk sizes** - use `DefaultSlinChunkSize`
4. **Don't send audio too fast** - let AudioSocket handle timing
5. **Don't ignore first chunk alignment** - incomplete first chunks cause 0.1s distortion
6. **Don't overcomplicate working systems** - make minimal, targeted fixes

### **Session Management:**
1. **Always close connections** with `defer conn.Close()`
2. **Use sync.WaitGroup** for goroutine management
3. **Handle shutdown gracefully** with channels
4. **Clean up resources** in `finalize()` method

### **Error Handling:**
1. **Wrap errors** with context: `fmt.Errorf("failed to %s: %w", action, err)`
2. **Log errors** with session ID for debugging
3. **Don't ignore errors** from audio operations

---

## ✅ BEST PRACTICES

### **Code Style:**
- **Use descriptive variable names** (`audioPlayer` not `ap`)
- **Add comments** for complex audio calculations
- **Log important events** (audio loaded, played, session started)
- **Use constants** for magic numbers

### **Performance:**
- **Pre-load audio files** at startup
- **Use buffered channels** for large data
- **Implement proper cleanup** for long-running operations

### **Testing:**
- **Test audio player** with empty directories
- **Verify WAV parsing** with different file formats
- **Test error conditions** (missing files, network issues)

---

## 🎓 LESSONS LEARNED (HARD WAY)

### **First Chunk Distortion Issue (2025-08-28):**
**Problem:** Brief 0.1 second distortion/click at the start of audio playback
**Root Cause:** First audio chunk not perfectly aligned to 320-byte boundaries
**Wrong Solution:** Completely rewriting working audio system (broke everything)
**Correct Solution:** Minimal fix to skip incomplete first chunks
**Lesson:** Never break what's already working - make surgical fixes only

### **Audio System Over-Engineering:**
**Problem:** Tried to fix 0.1s distortion by rewriting entire audio system
**Result:** Broke full audio playback (only 0.1s played)
**Lesson:** 
- Listen to user's exact problem description
- Make minimal, targeted changes
- Test small fixes before big changes
- Preserve existing working functionality

---

## 🔍 DEBUGGING CHECKLIST

### **Audio Issues:**
- [ ] Check chunk size (must be 320 bytes)
- [ ] Verify WAV format (8kHz, 16-bit, mono)
- [ ] Check audio file loading logs
- [ ] Verify first chunk alignment (no incomplete chunks at start)
- [ ] Check for 0.1s distortion at audio start (indicates chunk alignment issue)
- [ ] Verify `audiosocket.SendSlinChunks()` usage

### **Performance Issues:**
- [ ] Check goroutine leaks
- [ ] Verify proper cleanup in `finalize()`
- [ ] Monitor memory usage for large audio files
- [ ] Check network buffer sizes

---

## 📚 REFERENCE LINKS

### **AudioSocket Documentation:**
- **Chunk Size**: `audiosocket.DefaultSlinChunkSize = 320`
- **Send Function**: `audiosocket.SendSlinChunks(conn, size, data)`
- **Message Types**: `KindSlin`, `KindHangup`, `KindDTMF`

### **Audio Format:**
- **Sample Rate**: 8000 Hz (8kHz)
- **Bit Depth**: 16-bit signed
- **Channels**: 1 (Mono)
- **Chunk Duration**: 20ms

---

## 🚨 EMERGENCY FIXES

### **If Audio is Slow Motion:**
1. **Check chunk size** - must be 320 bytes
2. **Use `audiosocket.SendSlinChunks()`** - not custom implementation
3. **Remove custom tickers** - let AudioSocket handle timing
4. **Rebuild and test**

### **If Audio is Distorted:**
1. **Verify WAV parsing** - check data chunk offset
2. **Check sample rate** - must be 8000 Hz
3. **Verify bit depth** - must be 16-bit
4. **Check channel count** - must be mono

---

## 💡 REMEMBER THIS FOREVER

**🎵 AUDIO CHUNK SIZE = 320 BYTES (8000Hz × 20ms × 2 bytes) 🎵**

**Never use custom implementations when AudioSocket provides built-in functions!**

---

*Last Updated: 2025-08-28*
*Project: AudioSocket Transcriber*
*Critical Rule: Audio chunk size and timing*
