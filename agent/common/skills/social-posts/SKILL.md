---
name: social-posts
description: Use when drafting LinkedIn or X posts that promote a blog post, build log, or side project. Covers format templates, the one-visual rule and media specs, hooks, length, link placement, hashtags, X adaptation, and the author's voice rules. Not for long-form articles or the blog post itself.
---

# Social posts for build logs

Goal: a post that stands on its own in the feed and makes a technical reader want the full write-up.
Platform data conflicts between studies; rules below are the defaults most sources agree on (see Sources).

## Voice (non-negotiable)

- Plain first person. What I built, why, how it went, how it felt.
- Humour as asides (a parenthetical, a dry line), never the point of the post.
- Specific over general: real numbers, error messages, file names, versions.
- Banned AI tells: forced triads, "not X, but Y", one-line inspirational closers, "Here's the thing", "Let that sink in", "game-changer", "unlock", "dive in", emoji bullets or emoji spam, every sentence on its own line, rhetorical-question hooks, fake suspense ("I was wrong. Here's why 👇").
- No engagement bait: no "Comment YES for the link", no "Agree?", no tagging people who are not in the story.
- Never cite internal record numbers (decision/ticket ids); name the situation instead.
- Run the `humanizer` skill on the draft if anything sounds generic.

## Format templates (pick one)

1. **I built X, here's what broke**: hook = the concrete failure or surprise. Body: what I was building (1-2 lines), what broke, how I found it, the fix. Close: what I'd do differently + link pointer.
2. **Story -> insight -> takeaway**: a specific moment, what it taught, one practical takeaway the reader can use without clicking.
3. **Before / after**: before numbers or behaviour, the one change, after numbers. Visual: side-by-side.
4. **Listicle (only when the content is genuinely a list)**: "N things I learned building X", 3-7 numbered plain lines, each concrete.

Example hook: "My SQLite writes went from 40/s to 12,000/s by changing one pragma. I spent two days blaming the ORM first."

## Hook rules

- Feed truncates at "...see more": about 140 chars on mobile, about 210 chars / 3 lines on desktop with media. Write for 140.
- First line carries a concrete fact: a number, a failure, a named tool. No "Excited to share...".
- Line 2 earns the click on "see more".

## Length and structure

- LinkedIn: aim 800-1,500 characters (cap 3,000). Paragraphs of 1-3 sentences, blank line between. Not one sentence per line.
- Code only if 1-3 lines; otherwise on a slide.
- End with a plain pointer ("Full write-up: link in the first comment") or one genuine question. Not both, not a slogan.
- Tag only people or projects actually involved (max 1-3).

## Media (always ship one visual)

- **Document carousel (PDF)**: default for build logs on LinkedIn. 1080x1350 (4:5) or 1080x1080, 6-12 slides. Slide 1 = hook as title + one image; one idea per slide; large type; last slide = what's in the full post. Upload as PDF with a real title.
- **Single image**: diagram, before/after chart, terminal screenshot. 1200x627 or 1080x1350. Readable on a phone.
- **Multi-image** (2-4): before/after pairs or a short sequence.
- **GIF**: LinkedIn animates uploaded .gif in the feed. 3-8 s, under ~5 MB, cropped tight.
- **Native video**: real demos only; 30-90 s, captions burned in, result in the first 3 s.
- Polls: skip for build logs.

## Links

- LinkedIn has published no link penalty and data conflicts. Default: no link in the body; post the URL as the first comment yourself and say "link in the first comment".
- If the post is mainly a pointer, a body link is acceptable; make the body valuable anyway. No shorteners.

## Hashtags and timing

- 0-3 specific hashtags at the end, never generic.
- Tue-Thu, about 8-10 am in the audience's time zone (practitioner guidance, unverified). Reply to comments in the first 60-90 minutes. No edits in the first hour beyond typos.

## X adaptation

- 280 chars. Single post: hook + one concrete detail + visual. Thread (3-7) only for real steps; post 1 stands alone.
- Visual 1600x900 or 1200x1200; same image/GIF re-cropped.
- Links: X said (Oct 2025) it doesn't throttle links, but thin-text link posts underperform. Link in the main post only if the text stands alone, else first reply.
- 0-1 hashtags. No "A thread 🧵".

## Pre-publish checklist

- [ ] First 140 chars: a concrete fact that works alone.
- [ ] One visual attached, readable on a phone (carousel slide 1 = hook).
- [ ] No banned patterns.
- [ ] Every claim matches the blog post.
- [ ] First comment with the link drafted.
- [ ] 0-3 specific hashtags; tags only for involved people.
- [ ] X version under 280 chars with its own crop.
- [ ] The user publishes; never post on their behalf.

## Sources (checked 2026-11-08, from search summaries; spot-check numbers)

- van der Blom, Algorithm Insights 2025 (summary): https://www.writtenlyhub.com/news/linkedin-engagement-down-50-algorithm-insights-report-2025
- Buffer, best content format: https://buffer.com/resources/data-best-content-format-social-media/
- Socialinsider, 2025 LinkedIn benchmarks: https://socialinsider.io/social-media-benchmarks/linkedin
- AuthoredUp: https://authoredup.com/blog/best-performing-content-on-linkedin
- SEJ, 577k-post link study: https://www.searchenginejournal.com/linkedin-study-finds-adding-links-boosts-engagement-by-13/544524/
- Social Media Today, GIF support: https://www.socialmediatoday.com/news/linkedin-adds-gif-support/823229/
- Carousel specs: https://contentin.io/blog/linkedin-carousel-size/
- See-more cutoff: https://www.unilink.us/blog/linkedin-character-limits-2026
- X link penalty removed: https://ppc.land/x-drops-year-old-link-penalty-musk-tells-zuckerberg-on-platform/
