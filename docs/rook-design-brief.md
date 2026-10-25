# Rook — Mascot Design Brief

Rook is the mascot of **Groundskeeper**, Lot86's terminal tool for keeping a Mac's disk (and a connected iPhone) free of clutter. This brief sets out who Rook is, how Rook looks, and how to use Rook consistently across the tool, the README, reports and Lot86 Media videos.

## Character

**Who Rook is:** a raccoon who works as a groundskeeper. Raccoons are famous for rummaging through clutter; Rook does the opposite and tidies it up.

**Personality**
- **Diligent:** shows up twice a week, checks every corner and keeps notes.
- **Careful:** never throws anything away without asking first. This mirrors the tool's "recommend, then you act" design.
- **Friendly and a little proud of the work:** pleased by a tidy lot, never preachy about a messy one.
- **Unflappable:** a full disk is a job to do, not an emergency.

**Family:** Rook is the Lot86 sibling of Rafferty's otter. They share a flat, friendly style and a thick charcoal outline, so they look like they belong to the same studio.

## Visual design

### Silhouette and proportions

- **Head:** a wide oval, about 40% of total height in the full-body drawing, so the face stays readable at small sizes.
- **Ears:** pointed triangles with charcoal insides, peeking out from under the cap.
- **Body:** a rounded pear shape with a cream belly patch.
- **Tail:** large and bushy, angled up and away from the body, with **five charcoal rings** and a charcoal tip. The tail is the raccoon's second most recognizable feature after the mask, so it should be visible in every full-body pose.
- **Paws and feet:** solid charcoal. A raccoon's dark paws read as "gloves", which suits a working character.

### The face

| Feature | Description |
|---|---|
| Mask | A single charcoal band across both eyes, dipping slightly between them over the nose bridge. It must always be present and clearly visible. |
| Brow and cheeks | Cream area surrounding the mask, so the mask stands out |
| Eyes | White circles with charcoal pupils looking slightly inward, and a small white highlight. This keeps Rook friendly rather than sly. |
| Muzzle | Cream oval below the mask |
| Nose | Charcoal oval |
| Mouth | A small, closed "w" smile |

### The cap

- A **forest-green flat cap** with a slightly darker brim band, a dashed stitch line across the crown and a small button on top.
- The cap is Rook's signature item and the clearest link to the groundskeeper role. Keep it in every pose and at every size, including the icon.

### The rake

- An **amber wooden handle** with a charcoal rake head and five tines.
- In the standard pose, Rook holds it upright in the left paw with the head resting on the ground next to a small leaf pile.

### Line and shape rules

- **Outline:** charcoal `#2B2A28`, 6 px at the 256 px icon size, with round joins and caps.
- **Fills:** flat colour only. No gradients, textures or drop shadows.
- **Shapes:** built from ellipses and simple curves. Avoid fine detail that disappears below 64 px.
- **Ground:** a flat green ellipse under the feet in full-body art.

## Colour palette

### Core

| Role | Name | Hex |
|---|---|---|
| Cap and "Safe" UI items | Forest green | `#2F5D3A` |
| Rake handle and "Review" UI items | Amber | `#D9922E` |
| Fur | Warm grey | `#8A8178` |
| Mask, paws, outlines and "Keep" UI items | Charcoal | `#2B2A28` |
| Light fur, belly and light-mode background | Cream | `#F3EBDD` |

### Supporting

| Role | Hex |
|---|---|
| Cap crown (darker green) | `#1E3B25` |
| Cap brim | `#24482C` |
| Cap stitching and button | `#4E7F59` |
| Leaves | `#B8701E`, `#D9922E`, `#C9462B`, `#6F8F3A` |
| Eye whites and highlights | `#FFFFFF` |

### Contrast

- Charcoal outlines keep Rook readable on both cream (light mode) and dark backgrounds.
- On dark backgrounds darker than about `#333333`, place Rook on a cream or green disc rather than directly on the background, because the charcoal outline and mask would otherwise disappear.
- Never use colour alone to carry meaning in the UI. Safe, Review and Keep also get text labels.

## Poses

### Current

| Pose | File | Use |
|---|---|---|
| Standing with rake, paw on hip | `assets/rook.svg`, `assets/rook.png` | README header, report header, video thumbnails |
| Face icon on green disc | `assets/rook-face.svg`, `assets/rook-face.png` | Notification icon, favicon, app and channel avatars |

### Future ideas

| Pose | Matches |
|---|---|
| Peering through a magnifying glass | "Scanning…" state |
| Sweeping leaves into a bag | Cleanup in progress (`clean … --apply`) |
| Leaning on the rake, thumbs up | "All tidy!" success screen |
| Holding a clipboard | Report ready notification |
| Dozing against a tree | Nothing to report, or idle |
| Holding a phone with a cable | iPhone mode |
| Looking puzzled at a tangle of cables | Errors, such as "iPhone not trusted" |

New poses keep the cap, mask, ringed tail and charcoal paws, and use only the palette above.

## ASCII Rook (terminal)

Defined in `assets/rook-ascii.txt`, with named sections that the Go code embeds:

| Section | Use |
|---|---|
| `face` | Header of the photo review screen |
| `face-tiny` | `(@=@)` prefix in one-line messages |
| `full` | Rook with rake and leaves, for `--help` and the first-run welcome |
| `all-clean` | Success screen, with `{freed}`, `{free}` and `{disk}` placeholders |

**Rules**
- Plain ASCII only, so it renders in any terminal and font.
- The `=` between the eyes is the mask and must stay.
- Colour is optional and applied by the TUI, not stored in the file: green for the cap, amber for the rake handle, and default text colour for everything else. With `NO_COLOR` set, no colour is used.
- Keep every art section under 40 columns wide so it fits beside text in an 80-column terminal.

## Voice and tone

Rook's voice appears in notifications, the terminal UI and the report. It's warm, short and practical.

**Do**
- Lead with the useful number: "Rook found 12.4 GB to tidy up."
- Sound pleased by progress: "All tidy! Rook freed 6.2 GB today."
- Stay calm about problems: "Rook can't see your iPhone yet. Unlock it and tap Trust."
- Make the safe choice sound normal: "Nothing's deleted until you say so."

**Don't**
- Nag or shame: avoid "Your disk is a mess" or "You really should…".
- Overdo the character. One Rook mention per message is plenty, and error details stay plain and technical.
- Make jokes in confirmations about deleting things. Those messages are clear and serious.

**Sample lines**

| Situation | Line |
|---|---|
| Scheduled report ready | "Rook's report is ready: 118 GB free, about 35 GB to tidy." |
| Nothing to do | "All quiet on the lot. Nothing worth tidying today." |
| Free space growing | "Up 4.1 GB since Monday. Nice work." |
| Free space shrinking fast | "Heads up: 9 GB less free space since Thursday. Biggest change: `~/.ollama`." |
| Photo review start | "Rook found 23 groups of similar photos. The best shot in each is marked Keep." |
| Before deleting | "About to move 148 photos (≈ 2.1 GB) to Recently Deleted. Continue?" |

## Usage rules

**Do**
- Keep clear space around Rook equal to at least the width of the cap brim.
- Use the face icon at 16–64 px and the full body from 160 px up.
- Pair Rook with the `groundskeeper` wordmark in a monospace font.

**Don't**
- Stretch, skew, rotate or recolour Rook outside the palette.
- Remove the cap, the mask or the tail rings.
- Add outlines, glows, gradients or drop shadows.
- Place Rook on busy photographic backgrounds.
- Make Rook look sneaky or thieving, for example with a sack of loot or narrowed eyes. Rook tidies; Rook doesn't steal.

## Asset files

| File | Format | Size | Background |
|---|---|---|---|
| `assets/rook.svg` | SVG | 400 × 376 | Transparent |
| `assets/rook.png` | PNG | 800 px wide | Transparent |
| `assets/rook-face.svg` | SVG | 256 × 256 | Green disc |
| `assets/rook-face.png` | PNG | 512 × 512 | Green disc |
| `assets/rook-ascii.txt` | Text | Under 40 columns | — |

**Still to make**
- A dark-mode report header variant
- The `groundskeeper` wordmark lockup (Rook plus wordmark, horizontal and stacked)
- A macOS `.icns` app icon from `rook-face.svg`, for the notification icon
- A YouTube channel banner and thumbnail template for Lot86 Media
