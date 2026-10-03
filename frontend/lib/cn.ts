import { createCn } from "cn/config";

/**
 * Class merging that knows the design tokens (app/globals.css). Without this, the merger reads
 * `text-body` (a font size) and `text-white` (a color) as the same group and drops one of them.
 * Keep these lists in step with the `--text-*` and `--shadow-*` tokens.
 */
export const cn = createCn({
  extend: {
    classGroups: {
      "font-size": [
        {
          text: [
            "hero",
            "display",
            "heading-1",
            "heading-2",
            "heading-3",
            "body-lg",
            "body",
            "body-sm",
            "label",
            "overline",
            "figure-xl",
            "score-xl",
            "metric",
            "data",
          ],
        },
      ],
      shadow: [{ shadow: ["card", "overlay", "primary", "glow"] }],
    },
  },
});
