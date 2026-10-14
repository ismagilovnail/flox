"""Generates FLOX's system gallery images (generic, brand-free samples).

Usage, from the repo root (needs Pillow and the DejaVu fonts):
    python3 apps/internal/contentgallery/gen/make_assets.py apps/internal/contentgallery/system
Outside system/ on purpose: everything in system/ is embedded in the API binary.
"""
import os, sys
from PIL import Image, ImageDraw, ImageFont

OUT = sys.argv[1]
B = "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf"
R = "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"

def font(path, size): return ImageFont.truetype(path, size)

def gradient(w, h, top, bottom):
    img = Image.new("RGB", (w, h), top)
    d = ImageDraw.Draw(img)
    for y in range(h):
        t = y / max(h - 1, 1)
        c = tuple(int(top[i] + (bottom[i] - top[i]) * t) for i in range(3))
        d.line([(0, y), (w, y)], fill=c)
    return img

def hexc(s): s = s.lstrip("#"); return tuple(int(s[i:i+2], 16) for i in (0, 2, 4))

def centered(d, w, y, text, f, fill):
    tw = d.textlength(text, font=f)
    d.text(((w - tw) / 2, y), text, font=f, fill=fill)

def button(d, cx, y, text, f, bg, fg, padx=48, pady=22):
    tw = d.textlength(text, font=f)
    bb = f.getbbox(text)
    th = bb[3] - bb[1]
    x0, x1 = cx - tw / 2 - padx, cx + tw / 2 + padx
    d.rounded_rectangle([x0, y, x1, y + th + 2 * pady], radius=(th + 2 * pady) // 2, fill=bg)
    d.text((cx - tw / 2, y + pady - bb[1]), text, font=f, fill=fg)

def circles(d, w, h, color, seed):
    import random
    rnd = random.Random(seed)
    for _ in range(9):
        r = rnd.randint(w // 20, w // 9)
        # Keep the middle band, where the text sits, clear.
        x = rnd.choice([rnd.randint(-r // 2, int(w * 0.22)), rnd.randint(int(w * 0.78), w + r // 2)])
        y = rnd.randint(0, h)
        d.ellipse([x - r, y - r, x + r, y + r], outline=color, width=max(2, w // 300))

def save(img, rel):
    p = os.path.join(OUT, rel)
    os.makedirs(os.path.dirname(p), exist_ok=True)
    img = img.convert("P", palette=Image.ADAPTIVE, colors=128)
    img.save(p, optimize=True)

def creative(rel, w, h, top, bottom, accent, title, sub, cta, seed):
    img = gradient(w, h, hexc(top), hexc(bottom))
    d = ImageDraw.Draw(img)
    circles(d, w, h, hexc(accent), seed)
    s = min(w, h)
    centered(d, w, h * 0.30, title, font(B, s // 11), "white")
    centered(d, w, h * 0.30 + s // 7, sub, font(R, s // 24), (235, 235, 245))
    button(d, w / 2, h * 0.64, cta, font(B, s // 22), hexc(accent), "white")
    save(img, rel)

creative("creatives/install-banner.png", 1200, 628, "#1e3a8a", "#2563eb", "#f59e0b",
         "Get the app", "Free to install. Ready in seconds.", "Install now", 1)
creative("creatives/welcome-square.png", 1080, 1080, "#4c1d95", "#7c3aed", "#22c55e",
         "Welcome bonus", "Start with a little extra", "Claim it", 2)
creative("creatives/story-vertical.png", 720, 1280, "#0f172a", "#0e7490", "#f43f5e",
         "Swipe up", "Your new favourite app", "Open", 3)
creative("creatives/countdown-banner.png", 1200, 628, "#7f1d1d", "#ea580c", "#facc15",
         "Today only", "The offer ends at midnight", "Don't miss it", 4)

def logo(rel, bg, letter, fg="white", size=256):
    img = Image.new("RGB", (size, size), hexc(bg))
    d = ImageDraw.Draw(img)
    d.rounded_rectangle([size*0.12, size*0.12, size*0.88, size*0.88], radius=size//6, outline=fg, width=size//28)
    centered(d, size, size * 0.24, letter, font(B, int(size * 0.42)), fg)
    save(img, rel)

def hero(rel, top, bottom, accent, seed, w=960, h=540):
    img = gradient(w, h, hexc(top), hexc(bottom))
    d = ImageDraw.Draw(img)
    circles(d, w, h, hexc(accent), seed)
    save(img, rel)

logo("landings/cta/logo.png", "#2563eb", "A")
hero("landings/cta/hero.png", "#1e3a8a", "#3b82f6", "#93c5fd", 11)
logo("landings/quiz/logo.png", "#7c3aed", "Q")
logo("landings/countdown/logo.png", "#ea580c", "T")
hero("landings/countdown/hero.png", "#7f1d1d", "#f97316", "#fde68a", 13)

def app_icon(rel, top, bottom, glyph, size=512):
    img = gradient(size, size, hexc(top), hexc(bottom))
    d = ImageDraw.Draw(img)
    d.ellipse([size*0.18, size*0.18, size*0.82, size*0.82], outline="white", width=size//24)
    centered(d, size, size * 0.27, glyph, font(B, int(size * 0.38)), "white")
    save(img, rel)

def screenshot(rel, top, bottom, accent, title, lines, n, w=540, h=960):
    img = gradient(w, h, hexc(top), hexc(bottom))
    d = ImageDraw.Draw(img)
    centered(d, w, 90, title, font(B, 46), "white")
    y = 200
    for i, line in enumerate(lines):
        d.rounded_rectangle([40, y, w - 40, y + 120], radius=24, fill=hexc(accent) if i == n % len(lines) else (255, 255, 255, 40) and tuple(min(255, c + 30) for c in hexc(top)))
        d.text((70, y + 40), line, font=font(R, 30), fill="white")
        y += 150
    button(d, w / 2, h - 170, "Continue", font(B, 32), "white", hexc(top))
    save(img, rel)

app_icon("pwa/nova/icon.png", "#2563eb", "#22d3ee", "N")
for i, (title, lines) in enumerate([
        ("Daily rewards", ["Check in every day", "Collect your points", "Unlock new levels"]),
        ("Track progress", ["Your streak: 7 days", "Points this week", "Level 12 reached"]),
        ("Invite friends", ["Share your link", "Earn together", "Top the leaderboard"])]):
    screenshot(f"pwa/nova/screen-{i+1}.png", "#1d4ed8", "#0891b2", "#f59e0b", title, lines, i)

app_icon("pwa/pulse/icon.png", "#111827", "#7c3aed", "P")
for i, (title, lines) in enumerate([
        ("Play instantly", ["No download wait", "Works offline", "Light on battery"]),
        ("Weekly events", ["New challenges", "Seasonal themes", "Special prizes"]),
        ("Your profile", ["Achievements", "History", "Settings"])]):
    screenshot(f"pwa/pulse/screen-{i+1}.png", "#111827", "#4c1d95", "#22c55e", title, lines, i)
