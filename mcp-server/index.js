import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { z } from "zod";
import http from "http";
import fs from "fs";
import path from "path";

const API_BASE = process.env.VIDEO_EDITOR_API || "http://localhost:8090";
const MEDIA_HOST_PATH = process.env.VE_MEDIA_PATH || process.env.VE_MEDIA_HOST_PATH || "";

// ── HTTP helper ──────────────────────────────────────────────────────────────

const DEFAULT_TIMEOUT = 30_000; // 30s for normal calls
const LONG_TIMEOUT = 120_000;   // 2min for render/transcription

function apiCall(method, urlPath, body, timeout = DEFAULT_TIMEOUT) {
  return new Promise((resolve, reject) => {
    const url = new URL(urlPath, API_BASE);
    const opts = {
      method,
      hostname: url.hostname,
      port: url.port,
      path: url.pathname + url.search,
      headers: { "Content-Type": "application/json" },
      timeout,
    };
    const req = http.request(opts, (res) => {
      const chunks = [];
      res.on("data", (c) => chunks.push(c));
      res.on("end", () => {
        const raw = Buffer.concat(chunks);
        const ct = res.headers["content-type"] || "";
        if (ct.startsWith("image/")) {
          resolve({ _image: true, data: raw.toString("base64"), mimeType: ct });
        } else {
          try {
            resolve(JSON.parse(raw.toString()));
          } catch {
            resolve({ status: res.statusCode, body: raw.toString() });
          }
        }
      });
    });
    req.on("timeout", () => { req.destroy(); reject(new Error(`Request timed out after ${timeout}ms: ${method} ${urlPath}`)); });
    req.on("error", reject);
    if (body) req.write(JSON.stringify(body));
    req.end();
  });
}

function fetchImage(urlPath, timeout = DEFAULT_TIMEOUT) {
  return new Promise((resolve, reject) => {
    const url = new URL(urlPath, API_BASE);
    const opts = {
      hostname: url.hostname,
      port: url.port,
      path: url.pathname + url.search,
      timeout,
    };
    const req = http.get(opts, (res) => {
      const chunks = [];
      res.on("data", (c) => chunks.push(c));
      res.on("end", () => {
        if (res.statusCode < 200 || res.statusCode >= 300) {
          const body = Buffer.concat(chunks).toString();
          reject(new Error(`HTTP ${res.statusCode}: ${body}`));
          return;
        }
        resolve(Buffer.concat(chunks).toString("base64"));
      });
    });
    req.on("timeout", () => { req.destroy(); reject(new Error(`Image fetch timed out after ${timeout}ms: ${urlPath}`)); });
    req.on("error", reject);
  });
}

// ── Safe tool wrapper ───────────────────────────────────────────────────────

function safeTool(server, name, description, schema, handler) {
  server.tool(name, description, schema, async (params) => {
    try {
      return await handler(params);
    } catch (err) {
      return { content: [{ type: "text", text: `Error in ${name}: ${err.message}` }] };
    }
  });
}

// ── MCP Server ───────────────────────────────────────────────────────────────

const server = new McpServer({
  name: "video-editor",
  version: "1.0.0",
});

// ── Tool: Health Check ───────────────────────────────────────────────────────

safeTool(server,
  "health_check",
  "Check if the video editor API is running and get system info (ffmpeg version, etc.)",
  {},
  async () => {
    const result = await apiCall("GET", "/api/v1/health");
    return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
  }
);

// ── Tool: Create Project ─────────────────────────────────────────────────────

safeTool(server,
  "create_project",
  `Create a new video editing project. This is always the first step.
Returns the project JSON with a timeline_url you can fetch to see the empty timeline.
Default settings: 1920x1080, 30fps, yuv420p. Override with the settings parameter.
Common social media resolutions: 1080x1920 (Reels/TikTok/Shorts vertical), 1080x1080 (Instagram Square), 1920x1080 (YouTube/LinkedIn landscape).

Recommended editorial workflow — ALWAYS show visual previews to the user:
1. list_media_files / add_media — find or add source files
2. preview_frame — show the user candidate moments BEFORE selecting clips (ALWAYS display the image)
3. Add clips to timeline
4. generate_thumbnails — extract frames for visual previews
5. get_timeline_image — show the user the timeline layout (ALWAYS display this image in your response)
6. get_storyboard_image — show the user the sequence flow for approval (ALWAYS display this image — it's the "proof" before rendering)
7. User approves → submit_render

The MCP returns images inline — ALWAYS display them in your response so the user can see what the edit looks like. Never just describe an image in text.`,
  {
    name: z.string().describe("Project name"),
    width: z.number().optional().describe("Video width in pixels (default 1920)"),
    height: z.number().optional().describe("Video height in pixels (default 1080)"),
    fps: z.number().optional().describe("Frame rate (default 30)"),
  },
  async ({ name, width, height, fps }) => {
    const settings = {};
    if (width) settings.width = width;
    if (height) settings.height = height;
    if (fps) settings.frame_rate = { num: fps, den: 1 };
    settings.sample_rate = 48000;
    settings.pixel_format = "yuv420p";
    settings.color_space = "bt709";

    const body = { name, settings: Object.keys(settings).length > 0 ? settings : undefined };
    const result = await apiCall("POST", "/api/v1/projects", body);
    return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
  }
);

// ── Tool: List Projects ──────────────────────────────────────────────────────

safeTool(server,
  "list_projects",
  "List all existing video editing projects.",
  {},
  async () => {
    const result = await apiCall("GET", "/api/v1/projects");
    return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
  }
);

// ── Tool: Get Project ────────────────────────────────────────────────────────

safeTool(server,
  "get_project",
  "Get full project details including all assets and sequences.",
  { project_id: z.string().describe("Project UUID") },
  async ({ project_id }) => {
    const result = await apiCall("GET", `/api/v1/projects/${project_id}`);
    return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
  }
);

// ── Tool: Import Asset ───────────────────────────────────────────────────────

safeTool(server,
  "import_asset",
  `Import a media file into the project as an asset.
The file_path MUST use the /media/ prefix and match exactly what list_media_files returns (e.g. /media/recording.mp4).
Type is auto-detected from file extension, or specify: video, audio, image, subtitle.
After import, the asset gets an ID you'll use when adding clips to the timeline.
For Zoom recordings: view_type and recording_group are auto-detected from filenames. Import ALL available camera angles as separate assets so you can switch between them for different clips.`,
  {
    project_id: z.string().describe("Project UUID"),
    name: z.string().describe("Human-readable name for this asset (e.g. 'Speaker View', 'Gallery View')"),
    file_path: z.string().describe("Path to the media file inside the container (e.g. /media/recording.mp4)"),
    type: z.string().optional().describe("Asset type: video, audio, image, subtitle (auto-detected if omitted)"),
  },
  async ({ project_id, name, file_path, type }) => {
    const body = { name, file_path };
    if (type) body.type = type;
    const result = await apiCall("POST", `/api/v1/projects/${project_id}/assets`, body);
    return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
  }
);

// ── Tool: List Assets ────────────────────────────────────────────────────────

safeTool(server,
  "list_assets",
  "List all assets in a project. Optionally filter by type.",
  {
    project_id: z.string().describe("Project UUID"),
    type: z.string().optional().describe("Filter by type: video, audio, image, subtitle"),
  },
  async ({ project_id, type }) => {
    let url = `/api/v1/projects/${project_id}/assets`;
    if (type) url += `?type=${type}`;
    const result = await apiCall("GET", url);
    return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
  }
);

// ── Tool: Create Sequence ────────────────────────────────────────────────────

safeTool(server,
  "create_sequence",
  `Create a new editing sequence (timeline) in the project.
A sequence contains tracks, which contain clips. Think of it as an "edit" or "composition".
You can optionally set intro/outro title slides (with custom colors, fonts, and logos) and link a subtitle asset.
Import ALL available camera angles as assets and switch between them for different clips — this creates a more dynamic edit.
Returns the sequence JSON with storyboard_url for visual preview.`,
  {
    project_id: z.string().describe("Project UUID"),
    name: z.string().describe("Sequence name (e.g. 'Highlight Reel', 'Full Cut')"),
    subtitle_asset_id: z.string().optional().describe("Asset ID of a .vtt subtitle file to burn into the render"),
    intro_text: z.string().optional().describe("Text for the intro title slide"),
    intro_duration: z.number().optional().describe("Intro slide duration in seconds (default 3)"),
    intro_bg_color: z.string().optional().describe("Intro background color (default '0x1a1a2e'). Use hex like '0xFF5500' or named colors"),
    intro_font_family: z.string().optional().describe("Intro font family (e.g. 'DM Sans', 'Inter', 'Montserrat')"),
    intro_logo_asset_id: z.string().optional().describe("Asset ID of logo image to overlay on intro slide"),
    outro_text: z.string().optional().describe("Text for the outro title slide"),
    outro_duration: z.number().optional().describe("Outro slide duration in seconds (default 3)"),
    outro_bg_color: z.string().optional().describe("Outro background color (default '0x1a1a2e')"),
    outro_font_family: z.string().optional().describe("Outro font family"),
    outro_logo_asset_id: z.string().optional().describe("Asset ID of logo image to overlay on outro slide"),
    crop_mode: z.enum(["fit", "center_crop"]).optional().describe("How to handle aspect ratio mismatch. 'fit' (default) scales with black bars. 'center_crop' crops the center to fill the frame — ideal for vertical (9:16) Reels from horizontal (16:9) source video."),
  },
  async ({ project_id, name, subtitle_asset_id, intro_text, intro_duration, intro_bg_color, intro_font_family, intro_logo_asset_id, outro_text, outro_duration, outro_bg_color, outro_font_family, outro_logo_asset_id, crop_mode }) => {
    const body = { name };
    if (subtitle_asset_id) body.subtitle_asset_id = subtitle_asset_id;
    if (crop_mode) body.crop_mode = crop_mode;
    if (intro_text) {
      body.intro_slide = {
        text: intro_text,
        duration: intro_duration || 3,
        font_size: 42,
        font_color: "white",
        bg_color: intro_bg_color || "0x1a1a2e",
      };
      if (intro_font_family) body.intro_slide.font_family = intro_font_family;
      if (intro_logo_asset_id) body.intro_slide.logo_asset_id = intro_logo_asset_id;
    }
    if (outro_text) {
      body.outro_slide = {
        text: outro_text,
        duration: outro_duration || 3,
        font_size: 36,
        font_color: "white",
        bg_color: outro_bg_color || "0x1a1a2e",
      };
      if (outro_font_family) body.outro_slide.font_family = outro_font_family;
      if (outro_logo_asset_id) body.outro_slide.logo_asset_id = outro_logo_asset_id;
    }
    const result = await apiCall("POST", `/api/v1/projects/${project_id}/sequences`, body);
    return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
  }
);

// ── Tool: Create Sequences Batch ──────────────────────────────────────────────

safeTool(server,
  "create_sequences_batch",
  `Create multiple sequences at once from one source video — ideal for social media workflows.
Each sequence gets its own video track with clips auto-placed on the timeline.
Shared settings (branding, subtitles, crop mode) apply to all sequences unless overridden.
Saves 5-10 tool calls per clip compared to creating sequences individually.`,
  {
    project_id: z.string().describe("Project UUID"),
    sequences: z.array(z.object({
      name: z.string().describe("Sequence name"),
      clips: z.array(z.object({
        asset_id: z.string().describe("Asset UUID of the video"),
        source_in: z.number().describe("Start in source (seconds)"),
        source_out: z.number().describe("End in source (seconds)"),
      })).describe("Clips to add to the sequence"),
      intro_text: z.string().optional().describe("Intro slide text"),
      outro_text: z.string().optional().describe("Outro slide text"),
      subtitle_asset_id: z.string().optional().describe("Override subtitle asset"),
      crop_mode: z.enum(["fit", "center_crop"]).optional().describe("Override crop mode"),
    })).describe("Array of sequences to create"),
    shared_bg_color: z.string().optional().describe("Shared brand color for intro/outro slides"),
    shared_logo_asset_id: z.string().optional().describe("Shared logo asset for intro/outro slides"),
    shared_subtitle_asset_id: z.string().optional().describe("Shared subtitle asset for all sequences"),
    shared_crop_mode: z.enum(["fit", "center_crop"]).optional().describe("Default crop mode for all sequences"),
  },
  async ({ project_id, sequences, shared_bg_color, shared_logo_asset_id, shared_subtitle_asset_id, shared_crop_mode }) => {
    const body = {
      sequences: sequences.map((s) => {
        const seq = { name: s.name, clips: s.clips };
        if (s.subtitle_asset_id) seq.subtitle_asset_id = s.subtitle_asset_id;
        if (s.crop_mode) seq.crop_mode = s.crop_mode;
        if (s.intro_text) {
          seq.intro_slide = { text: s.intro_text, duration: 3, font_size: 42, font_color: "white", bg_color: shared_bg_color || "0x1a1a2e" };
          if (shared_logo_asset_id) seq.intro_slide.logo_asset_id = shared_logo_asset_id;
        }
        if (s.outro_text) {
          seq.outro_slide = { text: s.outro_text, duration: 3, font_size: 36, font_color: "white", bg_color: shared_bg_color || "0x1a1a2e" };
          if (shared_logo_asset_id) seq.outro_slide.logo_asset_id = shared_logo_asset_id;
        }
        return seq;
      }),
      shared_settings: {},
    };
    if (shared_bg_color) body.shared_settings.bg_color = shared_bg_color;
    if (shared_logo_asset_id) body.shared_settings.logo_asset_id = shared_logo_asset_id;
    if (shared_subtitle_asset_id) body.shared_settings.subtitle_asset_id = shared_subtitle_asset_id;
    if (shared_crop_mode) body.shared_settings.crop_mode = shared_crop_mode;
    const result = await apiCall("POST", `/api/v1/projects/${project_id}/sequences/batch`, body);
    return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
  }
);

// ── Tool: Add Track ──────────────────────────────────────────────────────────

safeTool(server,
  "add_track",
  `Add a track to a sequence. Tracks hold clips.
Type 'video' for video/image clips, 'audio' for audio clips.
You need at least one video track before adding clips.`,
  {
    project_id: z.string().describe("Project UUID"),
    sequence_id: z.string().describe("Sequence UUID"),
    name: z.string().describe("Track name (e.g. 'V1', 'A1')"),
    type: z.enum(["video", "audio"]).describe("Track type"),
  },
  async ({ project_id, sequence_id, name, type }) => {
    const body = { name, type, index: 0 };
    const result = await apiCall(
      "POST",
      `/api/v1/projects/${project_id}/sequences/${sequence_id}/tracks`,
      body
    );
    return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
  }
);

// ── Tool: Add Clip ───────────────────────────────────────────────────────────

safeTool(server,
  "add_clip",
  `Add a clip to a track on the timeline.
A clip references an asset and specifies:
- WHERE on the timeline it appears (timeline_in/timeline_out in seconds)
- WHICH PART of the source to use (source_in/source_out in seconds)

Example: To place seconds 60-90 of a video starting at timeline position 0:
  timeline_in=0, timeline_out=30, source_in=60, source_out=90

To use the ENTIRE video: source_in=0, source_out=<duration from asset metadata>.

TIP: You can use different assets for different clips in the same sequence. For example, use the speaker view asset for a 10-second soundbite, then cut to the gallery view for a reaction shot. Import all relevant video angles as assets and switch between them for a more dynamic edit.

After adding clips, use get_timeline_image to see the updated timeline.`,
  {
    project_id: z.string().describe("Project UUID"),
    sequence_id: z.string().describe("Sequence UUID"),
    track_id: z.string().describe("Track UUID"),
    asset_id: z.string().describe("Asset UUID (the source media)"),
    timeline_in: z.number().describe("Start position on timeline in seconds"),
    timeline_out: z.number().describe("End position on timeline in seconds"),
    source_in: z.number().describe("Start position in source media in seconds"),
    source_out: z.number().describe("End position in source media in seconds"),
  },
  async ({ project_id, sequence_id, track_id, asset_id, timeline_in, timeline_out, source_in, source_out }) => {
    const body = { asset_id, timeline_in, timeline_out, source_in, source_out, speed: 1 };
    const result = await apiCall(
      "POST",
      `/api/v1/projects/${project_id}/sequences/${sequence_id}/tracks/${track_id}/clips`,
      body
    );
    return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
  }
);

// ── Tool: Delete Clip ────────────────────────────────────────────────────────

safeTool(server,
  "delete_clip",
  "Remove a clip from the timeline. Returns the updated sequence with timeline visualization URLs.",
  {
    project_id: z.string().describe("Project UUID"),
    sequence_id: z.string().describe("Sequence UUID"),
    track_id: z.string().describe("Track UUID"),
    clip_id: z.string().describe("Clip UUID to remove"),
  },
  async ({ project_id, sequence_id, track_id, clip_id }) => {
    const result = await apiCall(
      "DELETE",
      `/api/v1/projects/${project_id}/sequences/${sequence_id}/tracks/${track_id}/clips/${clip_id}`
    );
    return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
  }
);

// ── Tool: Generate Thumbnails ────────────────────────────────────────────────

safeTool(server,
  "generate_thumbnails",
  `Extract thumbnail frames from all clips in a sequence.
Run this after adding clips and BEFORE calling get_timeline_image or get_storyboard_image, so those visualizations include actual video frames instead of blank placeholders. The user experience is much better with real thumbnails.
Returns a list of clip_id → thumbnail_url mappings.`,
  {
    project_id: z.string().describe("Project UUID"),
    sequence_id: z.string().describe("Sequence UUID"),
  },
  async ({ project_id, sequence_id }) => {
    const result = await apiCall(
      "POST",
      `/api/v1/projects/${project_id}/sequences/${sequence_id}/thumbnails`
    );
    return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
  }
);

// ── Tool: Get Timeline Image ─────────────────────────────────────────────────

safeTool(server,
  "get_timeline_image",
  `Get an NLE-style timeline visualization of the current sequence.
Shows tracks (V1, A1, etc.) with clip blocks, embedded thumbnails, and a timecode ruler.
Returns a PNG image. Run generate_thumbnails first for best results.

IMPORTANT: ALWAYS show the returned timeline image to the user after adding or modifying clips. This gives the user a visual overview of the edit — they can see clip placement, durations, and track layout at a glance. Show this after every significant timeline change (adding clips, reordering, etc.). Never just describe the timeline in text — display the image.`,
  {
    project_id: z.string().describe("Project UUID"),
    sequence_id: z.string().optional().describe("Sequence UUID (uses first sequence if omitted)"),
  },
  async ({ project_id, sequence_id }) => {
    const urlPath = sequence_id
      ? `/api/v1/projects/${project_id}/sequences/${sequence_id}/timeline.png`
      : `/api/v1/projects/${project_id}/timeline.png`;
    const b64 = await fetchImage(urlPath);
    return {
      content: [{ type: "image", data: b64, mimeType: "image/png" }],
    };
  }
);

// ── Tool: Get Storyboard Image ───────────────────────────────────────────────

safeTool(server,
  "get_storyboard_image",
  `Get a filmstrip-style storyboard of the sequence.
Shows a horizontal row of thumbnail frames: INTRO → clip1 → clip2 → ... → OUTRO.
Each frame shows the video thumbnail, asset name, and duration.
Run generate_thumbnails first for best results.

IMPORTANT: ALWAYS show the returned storyboard to the user. This is the most important approval checkpoint — show it BEFORE rendering so the user can review the sequence flow (intro → clips → outro) and approve or request changes. Think of this as the "proof" before printing. Never render without showing the storyboard first and getting user approval.`,
  {
    project_id: z.string().describe("Project UUID"),
    sequence_id: z.string().describe("Sequence UUID"),
  },
  async ({ project_id, sequence_id }) => {
    const b64 = await fetchImage(
      `/api/v1/projects/${project_id}/sequences/${sequence_id}/storyboard.png`
    );
    return {
      content: [{ type: "image", data: b64, mimeType: "image/png" }],
    };
  }
);

// ── Tool: Submit Render ──────────────────────────────────────────────────────

safeTool(server,
  "submit_render",
  `Submit a render job to produce the final video file.
This is the last step — it renders the sequence into an MP4 file.
The render runs asynchronously. Use get_render_status to poll for completion.
Available profiles: h264_high, h264_medium, h264_web, h265_high.`,
  {
    project_id: z.string().describe("Project UUID"),
    sequence_id: z.string().describe("Sequence UUID"),
    profile: z.string().optional().describe("Render profile name (default: h264_medium)"),
  },
  async ({ project_id, sequence_id, profile }) => {
    const body = { sequence_id };
    if (profile) body.profile_name = profile;
    const result = await apiCall("POST", `/api/v1/projects/${project_id}/renders`, body);
    return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
  }
);

// ── Tool: Get Render Status ──────────────────────────────────────────────────

safeTool(server,
  "get_render_status",
  `Check the status of a render job.
Status progression: queued → rendering → complete (or failed).
When complete, the output_path shows where the file is.
Use download_render to get the file.`,
  {
    project_id: z.string().describe("Project UUID"),
    render_id: z.string().describe("Render job UUID"),
  },
  async ({ project_id, render_id }) => {
    const result = await apiCall("GET", `/api/v1/projects/${project_id}/renders/${render_id}`);
    return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
  }
);

// ── Tool: List Render Profiles ───────────────────────────────────────────────

safeTool(server,
  "list_render_profiles",
  "List available render profiles (codec presets) for encoding the final video.",
  {},
  async () => {
    const result = await apiCall("GET", "/api/v1/render-profiles");
    return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
  }
);

// ── Tool: Get Sequence ───────────────────────────────────────────────────────

safeTool(server,
  "get_sequence",
  "Get full sequence details including all tracks and clips.",
  {
    project_id: z.string().describe("Project UUID"),
    sequence_id: z.string().describe("Sequence UUID"),
  },
  async ({ project_id, sequence_id }) => {
    const result = await apiCall("GET", `/api/v1/projects/${project_id}/sequences/${sequence_id}`);
    return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
  }
);

// ── Tool: List Media Files ────────────────────────────────────────────────

safeTool(server,
  "list_media_files",
  `List all media files available for import. This is typically the first thing you do.
Shows every file in the media directory with name, path, size, type, and for Zoom recordings: recording_group, view type, resolution, and description.
Use the 'path' field as the file_path when importing assets.

When multiple video files share the same recording_group, they are different camera angles of the same recording (e.g. Zoom meeting). For social media clips:
- Use the "speaker" (active speaker) view for talking-head moments and soundbites
- Use the "gallery" view for group discussions or reaction shots
- Use the "combined" view for full meeting context
You can mix views within a sequence — use different source assets for different clips on the timeline to cut between angles like a real video editor.`,
  {},
  async () => {
    const result = await apiCall("GET", "/api/v1/media");
    return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
  }
);

// ── Tool: Add Media ─────────────────────────────────────────────────────────

safeTool(server,
  "add_media",
  `Copy a file from the host machine into the media directory so it becomes visible to list_media_files and import_asset.
Use this when the user wants to edit a file that exists on their computer but isn't in the media directory yet (e.g. a new video recording, a logo image, a subtitle file).
The file is copied into the Docker bind-mount, so it appears in /media/ inside the container.
Requires the VE_MEDIA_PATH or VE_MEDIA_HOST_PATH environment variable to be set to the host media directory path.`,
  {
    host_path: z.string().describe("Absolute path to the file on the host machine (e.g. 'C:\\\\Users\\\\me\\\\Videos\\\\recording.mp4')"),
    filename: z.string().optional().describe("Rename the file inside /media/ (defaults to original filename)"),
  },
  async ({ host_path, filename }) => {
    if (!MEDIA_HOST_PATH) {
      return {
        content: [{
          type: "text",
          text: "Error: VE_MEDIA_PATH or VE_MEDIA_HOST_PATH environment variable is not set. Set it to the host directory that is bind-mounted as /media/ in Docker (the same path used in docker-compose.yml).",
        }],
      };
    }

    // Resolve source and destination
    const srcPath = path.resolve(host_path);
    const destName = filename || path.basename(srcPath);
    const destPath = path.join(MEDIA_HOST_PATH, destName);

    // Check source exists
    if (!fs.existsSync(srcPath)) {
      return {
        content: [{ type: "text", text: `Error: Source file not found: ${srcPath}` }],
      };
    }

    // Check not same file
    if (path.resolve(srcPath) === path.resolve(destPath)) {
      return {
        content: [{ type: "text", text: `File is already in the media directory: ${destName}` }],
      };
    }

    // Copy file
    try {
      fs.copyFileSync(srcPath, destPath);
      const stats = fs.statSync(destPath);
      const sizeMB = (stats.size / 1024 / 1024).toFixed(1);

      // Fetch updated file info from the API
      const mediaFiles = await apiCall("GET", "/api/v1/media");
      const fileInfo = Array.isArray(mediaFiles) ? mediaFiles.find((f) => f.name === destName) : null;

      if (fileInfo) {
        return { content: [{ type: "text", text: JSON.stringify(fileInfo, null, 2) }] };
      }
      return {
        content: [{
          type: "text",
          text: `Copied ${destName} (${sizeMB}MB) to media directory. Use list_media_files to see it.`,
        }],
      };
    } catch (err) {
      return {
        content: [{ type: "text", text: `Error copying file: ${err.message}` }],
      };
    }
  }
);

// ── Tool: Refresh Media ─────────────────────────────────────────────────────

safeTool(server,
  "refresh_media",
  `Re-scan the media directory and return the updated file list. Guaranteed fresh — no caching.
Use this after adding files to the project folder manually, or after using add_media, to verify the files are visible.
Returns the same data as list_media_files.`,
  {},
  async () => {
    const result = await apiCall("GET", "/api/v1/media");
    return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
  }
);

// ── Tool: Read Transcript ────────────────────────────────────────────────────

safeTool(server,
  "read_transcript",
  `Read the content of a VTT/SRT subtitle/transcript file.
Returns the raw transcript text so you can read it and identify interesting moments, topics, and good clip points.
Use optional start/end parameters (in seconds) to read only a specific time range.
Example: To read what was said between 10:00 and 15:00, use start=600, end=900.
The transcript has timestamps and speaker names — use this to decide where to cut.`,
  {
    project_id: z.string().describe("Project UUID"),
    asset_id: z.string().describe("Asset UUID of the subtitle/transcript file"),
    start: z.number().optional().describe("Start time in seconds to filter (e.g. 600 for 10:00)"),
    end: z.number().optional().describe("End time in seconds to filter (e.g. 900 for 15:00)"),
  },
  async ({ project_id, asset_id, start, end }) => {
    let url = `/api/v1/projects/${project_id}/assets/${asset_id}/transcript`;
    const params = [];
    if (start !== undefined) params.push(`start=${start}`);
    if (end !== undefined) params.push(`end=${end}`);
    if (params.length > 0) url += "?" + params.join("&");
    const result = await apiCall("GET", url);
    return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
  }
);

// ── Tool: Preview Frame ──────────────────────────────────────────────────────

safeTool(server,
  "preview_frame",
  `Extract and display a single video frame at a specific timestamp.
Returns a JPEG image. Great for checking the right view (speaker/gallery/combined) at a given moment.

IMPORTANT: ALWAYS show the returned image to the user. Use this tool before committing to a clip — show the user what the video looks like at the proposed start time so they can approve the shot selection. For social media clips, preview both the start and a mid-point frame. The user expects to see these previews as part of the editorial workflow. Never skip this step.`,
  {
    project_id: z.string().describe("Project UUID"),
    asset_id: z.string().describe("Asset UUID of the video file"),
    time: z.number().describe("Timestamp in seconds (e.g. 635 for 10:35)"),
  },
  async ({ project_id, asset_id, time }) => {
    const b64 = await fetchImage(
      `/api/v1/projects/${project_id}/assets/${asset_id}/frame?time=${time}`
    );
    return {
      content: [{ type: "image", data: b64, mimeType: "image/jpeg" }],
    };
  }
);

// ── Tool: Add Clips Batch ────────────────────────────────────────────────────

safeTool(server,
  "add_clips_batch",
  `Add multiple clips to a track in a single operation.
Much faster than adding clips one at a time. Thumbnails are auto-generated.
Each clip needs: asset_id, timeline_in, timeline_out, source_in, source_out.`,
  {
    project_id: z.string().describe("Project UUID"),
    sequence_id: z.string().describe("Sequence UUID"),
    track_id: z.string().describe("Track UUID"),
    clips: z.array(z.object({
      asset_id: z.string().describe("Asset UUID"),
      timeline_in: z.number().describe("Start on timeline (seconds)"),
      timeline_out: z.number().describe("End on timeline (seconds)"),
      source_in: z.number().describe("Start in source (seconds)"),
      source_out: z.number().describe("End in source (seconds)"),
    })).describe("Array of clip objects"),
  },
  async ({ project_id, sequence_id, track_id, clips }) => {
    const body = clips.map((c) => ({
      asset_id: c.asset_id,
      timeline_in: c.timeline_in,
      timeline_out: c.timeline_out,
      source_in: c.source_in,
      source_out: c.source_out,
      speed: 1,
    }));
    const result = await apiCall(
      "POST",
      `/api/v1/projects/${project_id}/sequences/${sequence_id}/tracks/${track_id}/clips/batch`,
      body
    );
    return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
  }
);

// ── Tool: Download Render ────────────────────────────────────────────────────

safeTool(server,
  "download_render",
  `Download a completed render. The render must be in 'complete' status.
Two modes:
1. save_to_media=true (DEFAULT, recommended): Copies the render into the /media/ directory inside Docker with the given filename. The file appears in the user's project folder automatically. Just provide a filename like "my_clip.mp4".
2. save_to_media=false: Downloads to a specific absolute path on the HOST machine. Use output_path for this (must be a valid absolute path on the host OS, e.g. C:/Users/me/Desktop/clip.mp4).`,
  {
    project_id: z.string().describe("Project UUID"),
    render_id: z.string().describe("Render job UUID"),
    save_to_media: z.boolean().optional().describe("Save to /media/ directory (default true). File appears in user's project folder."),
    filename: z.string().optional().describe("Filename when saving to media (e.g. 'my_clip.mp4')"),
    output_path: z.string().optional().describe("Absolute host path (only when save_to_media=false)"),
  },
  async ({ project_id, render_id, save_to_media, filename, output_path }) => {
    const shouldSaveToMedia = save_to_media !== false; // default true

    if (shouldSaveToMedia) {
      const fname = filename || `render_${render_id}.mp4`;
      const result = await apiCall(
        "POST",
        `/api/v1/projects/${project_id}/renders/${render_id}/copy-to-media`,
        { filename: fname }
      );
      if (result.error) {
        return { content: [{ type: "text", text: `Error: ${result.error}` }] };
      }
      return { content: [{ type: "text", text: `Saved to media directory as ${result.filename || fname} (${result.size_mb || '?'}MB). File is now in the user's project folder.` }] };
    }

    // Legacy: download to host path
    if (!output_path) {
      return { content: [{ type: "text", text: "output_path is required when save_to_media is false" }] };
    }
    const job = await apiCall("GET", `/api/v1/projects/${project_id}/renders/${render_id}`);
    if (job.status !== "complete") {
      return { content: [{ type: "text", text: `Render not complete. Status: ${job.status}` }] };
    }
    return new Promise((resolve, reject) => {
      const url = new URL(`/api/v1/projects/${project_id}/renders/${render_id}/download`, API_BASE);
      http.get(url, (res) => {
        const fileStream = fs.createWriteStream(output_path);
        res.pipe(fileStream);
        fileStream.on("finish", () => {
          fileStream.close();
          const stats = fs.statSync(output_path);
          const sizeMB = (stats.size / 1024 / 1024).toFixed(1);
          resolve({
            content: [{ type: "text", text: `Downloaded to ${output_path} (${sizeMB}MB)` }],
          });
        });
        fileStream.on("error", reject);
      });
    });
  }
);

// ── Tool: Add Overlay (Image/Logo Watermark) ────────────────────────────────

safeTool(server,
  "add_overlay",
  `Add an image overlay (logo, watermark) on top of the video.
The image is composited over the entire sequence (or a time range).
Import the image as an asset first (PNG with transparency works best).
Position presets: top_left, top_right, bottom_left, bottom_right, center.
For custom positioning, use position='custom' with x,y coordinates.`,
  {
    project_id: z.string().describe("Project UUID"),
    sequence_id: z.string().describe("Sequence UUID"),
    asset_id: z.string().describe("Asset UUID of the image to overlay (must be type 'image')"),
    position: z.enum(["top_left", "top_right", "bottom_left", "bottom_right", "center", "custom"]).optional().describe("Where to place the overlay (default: top_right)"),
    x: z.number().optional().describe("Custom X coordinate (when position='custom')"),
    y: z.number().optional().describe("Custom Y coordinate (when position='custom')"),
    width: z.string().optional().describe("Width in pixels ('150') or percentage of frame ('15%'). Height auto-scales to maintain aspect ratio."),
    opacity: z.number().optional().describe("Opacity 0.0-1.0 (default 1.0)"),
    start_time: z.number().optional().describe("When to start showing overlay in seconds (default: start of sequence)"),
    end_time: z.number().optional().describe("When to stop showing overlay in seconds (default: end of sequence)"),
    padding: z.number().optional().describe("Pixels from edge for position presets (default 20)"),
  },
  async ({ project_id, sequence_id, asset_id, position, x, y, width, opacity, start_time, end_time, padding }) => {
    const body = { asset_id };
    if (position) body.position = position;
    if (x !== undefined) body.x = x;
    if (y !== undefined) body.y = y;
    if (width) body.width = width;
    if (opacity !== undefined) body.opacity = opacity;
    if (start_time !== undefined) body.start_time = start_time;
    if (end_time !== undefined) body.end_time = end_time;
    if (padding !== undefined) body.padding = padding;
    const result = await apiCall("POST", `/api/v1/projects/${project_id}/sequences/${sequence_id}/overlays`, body);
    return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
  }
);

// ── Tool: Add Text Overlay ──────────────────────────────────────────────────

safeTool(server,
  "add_text_overlay",
  `Burn text on top of the video (lower-thirds, captions, call-to-action, titles).
The text appears during a specified time range. You can style the font, add a background box, and control positioning.
Available fonts (bundled in Docker): DM Sans, DM Serif Display, Inter, Roboto, Open Sans, Montserrat.`,
  {
    project_id: z.string().describe("Project UUID"),
    sequence_id: z.string().describe("Sequence UUID"),
    text: z.string().describe("Text to display"),
    position: z.enum(["top", "center", "bottom", "custom"]).optional().describe("Where to place text (default: bottom)"),
    x: z.number().optional().describe("Custom X coordinate (when position='custom')"),
    y: z.number().optional().describe("Custom Y coordinate (when position='custom')"),
    font_family: z.string().optional().describe("Font name (default 'DM Sans'). Available: DM Sans, DM Serif Display, Inter, Roboto, Open Sans, Montserrat"),
    font_size: z.number().optional().describe("Font size in pixels (default 36)"),
    font_color: z.string().optional().describe("Text color as hex (default '#FFFFFF')"),
    bold: z.boolean().optional().describe("Bold text"),
    bg_color: z.string().optional().describe("Background box color as hex (e.g. '#0067B1' for a branded lower-third)"),
    bg_opacity: z.number().optional().describe("Background opacity 0.0-1.0 (default 0.8)"),
    padding: z.number().optional().describe("Box padding in pixels (default 10)"),
    start_time: z.number().describe("When to start showing text (seconds)"),
    end_time: z.number().describe("When to stop showing text (seconds)"),
    animation: z.enum(["fade_in", "slide_up", "none"]).optional().describe("Text animation (default 'none')"),
  },
  async ({ project_id, sequence_id, text, position, x, y, font_family, font_size, font_color, bold, bg_color, bg_opacity, padding, start_time, end_time, animation }) => {
    const body = { text, start_time, end_time };
    if (position) body.position = position;
    if (x !== undefined) body.x = x;
    if (y !== undefined) body.y = y;
    if (font_family) body.font_family = font_family;
    if (font_size) body.font_size = font_size;
    if (font_color) body.font_color = font_color;
    if (bold) body.bold = bold;
    if (bg_color) body.bg_color = bg_color;
    if (bg_opacity !== undefined) body.bg_opacity = bg_opacity;
    if (padding !== undefined) body.padding = padding;
    if (animation) body.animation = animation;
    const result = await apiCall("POST", `/api/v1/projects/${project_id}/sequences/${sequence_id}/text-overlays`, body);
    return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
  }
);

// ── Tool: Set Crop ──────────────────────────────────────────────────────────

safeTool(server,
  "set_crop",
  `Set crop/zoom on a clip. Essential for reframing landscape (16:9) video to vertical (9:16) for social media.
Modes:
- center_crop: Auto-crops from center to fill the project resolution (great for landscape→vertical)
- speaker_focus_left: Crops to left half of frame (for Zoom gallery views, isolate left speaker)
- speaker_focus_right: Crops to right half of frame
- Manual: Provide crop_x, crop_y, crop_width, crop_height for precise control`,
  {
    project_id: z.string().describe("Project UUID"),
    sequence_id: z.string().describe("Sequence UUID"),
    track_id: z.string().describe("Track UUID"),
    clip_id: z.string().describe("Clip UUID"),
    mode: z.enum(["center_crop", "speaker_focus_left", "speaker_focus_right"]).optional().describe("Auto crop mode"),
    crop_x: z.number().optional().describe("Manual: top-left X of crop region in source pixels"),
    crop_y: z.number().optional().describe("Manual: top-left Y of crop region"),
    crop_width: z.number().optional().describe("Manual: width of crop region"),
    crop_height: z.number().optional().describe("Manual: height of crop region"),
  },
  async ({ project_id, sequence_id, track_id, clip_id, mode, crop_x, crop_y, crop_width, crop_height }) => {
    const crop = {};
    if (mode) crop.mode = mode;
    if (crop_x !== undefined) crop.crop_x = crop_x;
    if (crop_y !== undefined) crop.crop_y = crop_y;
    if (crop_width !== undefined) crop.crop_width = crop_width;
    if (crop_height !== undefined) crop.crop_height = crop_height;
    const result = await apiCall(
      "PATCH",
      `/api/v1/projects/${project_id}/sequences/${sequence_id}/tracks/${track_id}/clips/${clip_id}`,
      { crop }
    );
    return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
  }
);

// ── Tool: Set Transition ────────────────────────────────────────────────────

safeTool(server,
  "set_transition",
  `Set the transition effect INTO a clip. Controls how this clip blends with the previous clip/slide.
Default is a 1-second fade. Set type='none' to cut directly with no transition.
Types: fade, dissolve, wipe_left, wipe_right, slide_left, slide_right, none.`,
  {
    project_id: z.string().describe("Project UUID"),
    sequence_id: z.string().describe("Sequence UUID"),
    track_id: z.string().describe("Track UUID"),
    clip_id: z.string().describe("Clip UUID"),
    type: z.enum(["fade", "dissolve", "wipe_left", "wipe_right", "slide_left", "slide_right", "none"]).describe("Transition type"),
    duration: z.number().optional().describe("Transition duration in seconds (default 1.0)"),
  },
  async ({ project_id, sequence_id, track_id, clip_id, type: transType, duration }) => {
    // Map friendly names to model transition types
    const typeMap = {
      fade: "fade",
      dissolve: "dissolve",
      wipe_left: "wipe",
      wipe_right: "wipe",
      slide_left: "slide",
      slide_right: "slide",
      none: "none",
    };
    const paramsMap = {
      wipe_right: { direction: "right" },
      slide_right: { direction: "right" },
    };

    const transition_in = {
      type: typeMap[transType] || "fade",
      duration: duration || 1.0,
    };
    if (paramsMap[transType]) transition_in.params = paramsMap[transType];

    const result = await apiCall(
      "PATCH",
      `/api/v1/projects/${project_id}/sequences/${sequence_id}/tracks/${track_id}/clips/${clip_id}`,
      { transition_in }
    );
    return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
  }
);

// ── Tool: Set Subtitle Style ────────────────────────────────────────────────

safeTool(server,
  "set_subtitle_style",
  `Control the appearance of burned-in subtitles for branded content.
Applies to the subtitle file linked to the sequence via subtitle_asset_id.
Default ffmpeg subtitles are plain white — use this to match brand styling.
Important: set margin_bottom to at least 50 for Reels/Shorts where the bottom is covered by UI.`,
  {
    project_id: z.string().describe("Project UUID"),
    sequence_id: z.string().describe("Sequence UUID"),
    font_family: z.string().optional().describe("Font name (default 'DM Sans Bold')"),
    font_size: z.number().optional().describe("Font size (default 24)"),
    font_color: z.string().optional().describe("Text color as hex (default '#FFFFFF')"),
    outline_color: z.string().optional().describe("Outline color as hex (default '#000000')"),
    outline_width: z.number().optional().describe("Outline width in pixels (default 2)"),
    bg_color: z.string().optional().describe("Background box color as hex (e.g. '#000000')"),
    bg_opacity: z.number().optional().describe("Background opacity 0.0-1.0"),
    position: z.enum(["top", "center", "bottom"]).optional().describe("Subtitle position (default 'bottom')"),
    margin_bottom: z.number().optional().describe("Pixels from bottom edge (default 50, important for Reels)"),
  },
  async ({ project_id, sequence_id, font_family, font_size, font_color, outline_color, outline_width, bg_color, bg_opacity, position, margin_bottom }) => {
    const style = {};
    if (font_family) style.font_family = font_family;
    if (font_size) style.font_size = font_size;
    if (font_color) style.font_color = font_color;
    if (outline_color) style.outline_color = outline_color;
    if (outline_width !== undefined) style.outline_width = outline_width;
    if (bg_color) style.bg_color = bg_color;
    if (bg_opacity !== undefined) style.bg_opacity = bg_opacity;
    if (position) style.position = position;
    if (margin_bottom !== undefined) style.margin_bottom = margin_bottom;
    const result = await apiCall("PUT", `/api/v1/projects/${project_id}/sequences/${sequence_id}/subtitle-style`, style);
    return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
  }
);

// ── Tool: Suggest Clips ─────────────────────────────────────────────────────

safeTool(server,
  "suggest_clips",
  `Analyze a transcript and suggest clip-worthy moments.
Saves you from reading the entire transcript. Returns suggested clips ranked by engagement potential, with start/end times, preview text, thumbnail frames, and reasons why each moment would make a good clip.
Uses heuristics: sentence completeness, energy markers (!?), strong language, topic breaks.
Each suggestion includes a thumbnail image from the video at the clip's start time.`,
  {
    project_id: z.string().describe("Project UUID"),
    asset_id: z.string().describe("Asset UUID of the subtitle/transcript file"),
    max_clips: z.number().optional().describe("Maximum suggestions to return (default 5)"),
    min_duration: z.number().optional().describe("Minimum clip duration in seconds (default 15)"),
    max_duration: z.number().optional().describe("Maximum clip duration in seconds (default 120)"),
    video_asset_id: z.string().optional().describe("Asset UUID of the video file (for thumbnail extraction). Auto-detected if omitted."),
    topic: z.string().optional().describe("Filter suggestions by topic (e.g. 'recruiting', 'social media tips'). Only segments mentioning these keywords will be considered."),
  },
  async ({ project_id, asset_id, max_clips, min_duration, max_duration, video_asset_id, topic }) => {
    const params = [];
    if (max_clips) params.push(`max=${max_clips}`);
    if (min_duration) params.push(`min_duration=${min_duration}`);
    if (max_duration) params.push(`max_duration=${max_duration}`);
    if (video_asset_id) params.push(`video_asset_id=${video_asset_id}`);
    if (topic) params.push(`topic=${encodeURIComponent(topic)}`);
    let url = `/api/v1/projects/${project_id}/assets/${asset_id}/suggest-clips`;
    if (params.length > 0) url += "?" + params.join("&");
    const result = await apiCall("GET", url);
    return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
  }
);

// ── Tool: Transcribe Asset ───────────────────────────────────────────────────

safeTool(server,
  "transcribe_asset",
  `Generate a VTT transcript from a video or audio asset using OpenAI Whisper.
This is essential when the recording doesn't come with a transcript file.
Runs asynchronously — use get_transcription_status to poll for completion.
The completed transcript is auto-imported as a subtitle asset you can then attach to a sequence.
GPU acceleration (CUDA) is used automatically when available, dramatically speeding up transcription.
Model options: tiny (fastest, least accurate), base (good default), small, medium, large (slowest, most accurate).
For a 1-hour video: tiny ~1min on GPU / ~10min CPU, base ~2min / ~20min, medium ~5min / ~45min.`,
  {
    project_id: z.string().describe("Project UUID"),
    asset_id: z.string().describe("Asset UUID of the video or audio file to transcribe"),
    model: z.enum(["tiny", "base", "small", "medium", "large"]).optional()
      .describe("Whisper model size (default: 'base'). Larger = more accurate but slower."),
    language: z.string().optional()
      .describe("Language code (e.g. 'en', 'es', 'fr'). Auto-detected if omitted."),
  },
  async ({ project_id, asset_id, model, language }) => {
    const body = { asset_id };
    if (model) body.model = model;
    if (language) body.language = language;
    const result = await apiCall("POST", `/api/v1/projects/${project_id}/transcriptions`, body);
    return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
  }
);

// ── Tool: Get Transcription Status ──────────────────────────────────────────

safeTool(server,
  "get_transcription_status",
  `Check the status of a transcription job.
Statuses: queued → extracting_audio → transcribing → complete / failed.
When complete, output_asset_id contains the subtitle asset UUID that can be attached to a sequence via subtitle_asset_id.`,
  {
    project_id: z.string().describe("Project UUID"),
    job_id: z.string().describe("Transcription job UUID returned by transcribe_asset"),
  },
  async ({ project_id, job_id }) => {
    const result = await apiCall("GET", `/api/v1/projects/${project_id}/transcriptions/${job_id}`);
    return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
  }
);

// ── Tool: List Transcriptions ───────────────────────────────────────────────

safeTool(server,
  "list_transcriptions",
  "List all transcription jobs for a project.",
  {
    project_id: z.string().describe("Project UUID"),
  },
  async ({ project_id }) => {
    const result = await apiCall("GET", `/api/v1/projects/${project_id}/transcriptions`);
    return { content: [{ type: "text", text: JSON.stringify(result, null, 2) }] };
  }
);

// ── Error handlers ──────────────────────────────────────────────────────────

process.on("unhandledRejection", (err) => {
  console.error("Unhandled rejection:", err);
});
process.on("uncaughtException", (err) => {
  console.error("Uncaught exception:", err);
});

// ── Start ────────────────────────────────────────────────────────────────────

const transport = new StdioServerTransport();
await server.connect(transport);
