import { describe, expect, it } from "vitest";

import { formatDuration } from "@/features/content-gallery/gallery-file-card";
import { isVideoFile, needsTranscoding, POSTER_MAX_SIDE, posterSize } from "@/lib/video-poster";

describe("gallery video helpers", () => {
  it("recognises MP4/WebM by type or extension", () => {
    expect(isVideoFile(new File([], "clip.MP4"))).toBe(true);
    expect(isVideoFile(new File([], "loop", { type: "video/webm" }))).toBe(true);
    expect(isVideoFile(new File([], "pic.png", { type: "image/png" }))).toBe(false);
  });

  it("leaves MOV and other converted formats to the server", () => {
    expect(needsTranscoding(new File([], "IMG_0001.MOV", { type: "video/quicktime" }))).toBe(true);
    expect(needsTranscoding(new File([], "clip.mkv"))).toBe(true);
    expect(needsTranscoding(new File([], "clip.mp4", { type: "video/mp4" }))).toBe(false);
    expect(needsTranscoding(new File([], "loop.webm"))).toBe(false);
  });

  it("scales the poster down to its longest side, never up", () => {
    expect(posterSize(1080, 1920)).toEqual({ width: 360, height: POSTER_MAX_SIDE });
    expect(posterSize(1920, 1080)).toEqual({ width: POSTER_MAX_SIDE, height: 360 });
    expect(posterSize(320, 240)).toEqual({ width: 320, height: 240 });
  });

  it("formats durations as m:ss", () => {
    expect(formatDuration(15300)).toBe("0:15");
    expect(formatDuration(61_000)).toBe("1:01");
    expect(formatDuration(600_000)).toBe("10:00");
  });
});
