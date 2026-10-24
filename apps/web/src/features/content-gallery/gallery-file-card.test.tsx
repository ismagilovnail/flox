import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";

import "@/lib/i18n/config";
import { GalleryFileCard } from "@/features/content-gallery/gallery-file-card";
import type { GalleryFile } from "@/lib/api/content-gallery";

// Transcoding (docs/content-gallery.md): a video being converted or that
// failed shows its state and cannot be played.
const video = (over: Partial<GalleryFile>): GalleryFile => ({
  id: "f1",
  organizationId: "o1",
  folderId: null,
  uploadedByUserId: "u1",
  fileType: "video",
  originalFilename: "IMG_0001.MOV",
  mimeType: "video/quicktime",
  sizeBytes: 1 << 20,
  status: "active",
  createdAt: "2026-10-24T00:00:00Z",
  updatedAt: "2026-10-24T00:00:00Z",
  url: "",
  processingStatus: "ready",
  ...over,
});

const card = (file: GalleryFile) =>
  render(<GalleryFileCard file={file} folders={[]} selected={false} onToggleSelect={() => {}} onPreview={() => {}} />);

describe("GalleryFileCard", () => {
  it("shows a video being converted, without a play button", () => {
    card(video({ processingStatus: "processing" }));
    expect(screen.getByText("Converting…")).toBeTruthy();
    expect(screen.queryByLabelText(/^Play/)).toBeNull();
  });

  it("shows why a conversion failed", () => {
    card(video({ processingStatus: "failed", processingError: "the file has no video stream" }));
    expect(screen.getByText("Couldn't convert")).toBeTruthy();
    expect(screen.getByText("the file has no video stream")).toBeTruthy();
    expect(screen.queryByLabelText(/^Play/)).toBeNull();
  });

  it("plays a ready video", () => {
    card(video({ mimeType: "video/mp4", url: "https://s3.example/f1.mp4", durationMs: 4200 }));
    expect(screen.getByLabelText("Play IMG_0001.MOV")).toBeTruthy();
    expect(screen.getByText("0:04")).toBeTruthy();
  });
});
