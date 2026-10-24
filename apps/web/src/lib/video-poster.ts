/**
 * What the browser reads from a video before it is uploaded to the Content
 * Gallery (docs/content-gallery.md "Video"): its first frame as a poster
 * image, its duration and frame size. FLOX stores MP4/WebM as uploaded, so
 * this is where their poster comes from; formats the server converts
 * (TRANSCODE_EXTENSIONS) get theirs from apps/worker instead. Any
 * failure (a codec this browser cannot decode, a timeout) yields what was
 * read so far: the video still uploads, just without a poster.
 */

export type VideoInfo = {
  poster?: Blob;
  durationMs?: number;
  width?: number;
  height?: number;
};

/** The poster's longest side, in pixels: a thumbnail, not a still. */
export const POSTER_MAX_SIDE = 640;
const TIMEOUT_MS = 8000;

export const VIDEO_EXTENSIONS = [".mp4", ".webm"];
/** Converted to MP4 on the server (docs/content-gallery.md "Transcoding"). */
export const TRANSCODE_EXTENSIONS = [".mov", ".m4v", ".avi", ".mkv", ".3gp"];

/** A video the server converts: the browser does not read it (it may not
 * decode it, and the server makes the poster anyway). */
export function needsTranscoding(file: File): boolean {
  const name = file.name.toLowerCase();
  return TRANSCODE_EXTENSIONS.some((ext) => name.endsWith(ext));
}

export function isVideoFile(file: File): boolean {
  const name = file.name.toLowerCase();
  return file.type.startsWith("video/") || VIDEO_EXTENSIONS.some((ext) => name.endsWith(ext));
}

/** The poster's size for a frame of width × height. */
export function posterSize(width: number, height: number): { width: number; height: number } {
  const scale = Math.min(1, POSTER_MAX_SIDE / Math.max(width, height));
  return { width: Math.max(1, Math.round(width * scale)), height: Math.max(1, Math.round(height * scale)) };
}

export function readVideoInfo(file: File): Promise<VideoInfo> {
  return new Promise((resolve) => {
    const info: VideoInfo = {};
    const url = URL.createObjectURL(file);
    const video = document.createElement("video");
    let done = false;
    const finish = () => {
      if (done) return;
      done = true;
      clearTimeout(timer);
      video.removeAttribute("src");
      video.load();
      URL.revokeObjectURL(url);
      resolve(info);
    };
    const timer = setTimeout(finish, TIMEOUT_MS);

    video.muted = true;
    video.playsInline = true;
    video.preload = "auto";
    video.onerror = finish;
    video.onloadedmetadata = () => {
      if (Number.isFinite(video.duration) && video.duration > 0) info.durationMs = Math.round(video.duration * 1000);
      if (video.videoWidth > 0 && video.videoHeight > 0) {
        info.width = video.videoWidth;
        info.height = video.videoHeight;
      }
      // A frame a little in: the very first one is often black.
      video.currentTime = Math.min(0.5, (video.duration || 1) / 2);
    };
    video.onseeked = () => {
      if (!info.width || !info.height) return finish();
      const size = posterSize(info.width, info.height);
      const canvas = document.createElement("canvas");
      canvas.width = size.width;
      canvas.height = size.height;
      const ctx = canvas.getContext("2d");
      if (!ctx) return finish();
      ctx.drawImage(video, 0, 0, size.width, size.height);
      canvas.toBlob(
        (blob) => {
          if (blob) info.poster = blob;
          finish();
        },
        "image/jpeg",
        0.8,
      );
    };
    video.src = url;
  });
}
