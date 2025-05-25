import { GoogleGenAI } from "@google/genai";
import { Hono } from "hono";
import RSS from "rss";

type Contributor = {
  avatar: string;
  name: string;
  url: string;
};

type TrendingRepo = {
  title: string;
  url: string;
  description: string;
  language?: string;
  languageColor?: string;
  stars: string;
  forks: string;
  addStars: string;
  contributors: Contributor[];
};

export type TrendingRepoWithSummary = TrendingRepo & {
  summary: string;
};

type TrendingData = {
  items: TrendingRepoWithSummary[];
  generatedAt: string;
};

const app = new Hono<{ Bindings: Env }>();

const KV_KEY = "github-trending-data";

const fetchTrending = async (): Promise<TrendingRepo[]> => {
  const url =
    "https://raw.githubusercontent.com/isboyjc/github-trending-api/main/data/daily/all.json";
  const res = await fetch(url);

  if (!res.ok) {
    throw new Error("Failed to fetch trending data");
  }

  const data = (await res.json()) as { items?: TrendingRepo[] };

  if (!data?.items || !Array.isArray(data.items)) {
    throw new Error("Invalid data format");
  }

  return data.items;
};

const fetchReadme = async (
  owner: string,
  name: string,
): Promise<string | null> => {
  const branches = ["main", "master"];

  for (const branch of branches) {
    const url = `https://raw.githubusercontent.com/${owner}/${name}/${branch}/README.md`;
    try {
      const res = await fetch(url);
      if (res.ok) {
        return await res.text();
      }
    } catch {}
  }

  return null;
};

const summarizeReadme = async (
  readme: string,
  apiKey: string,
): Promise<string> => {
  const ai = new GoogleGenAI({ apiKey });

  try {
    const response = await ai.models.generateContent({
      model: "gemini-2.0-flash",
      contents: `以下のREADMEの内容を日本語で短く要約せよ。100文字以内で\n\n${readme}`,
    });
    return response.text || "要約失敗";
  } catch {
    return "要約失敗";
  }
};

const getTrendingReposWithSummary = async (
  apiKey: string,
): Promise<TrendingRepoWithSummary[]> => {
  const repos = await fetchTrending();

  const reposWithSummary = await Promise.all(
    repos.map(async (repo) => {
      const [owner, name] = repo.title.split("/");
      const readme = await fetchReadme(owner, name);

      let summary = "要約失敗";
      if (readme) {
        summary = await summarizeReadme(readme, apiKey);
      }

      return {
        ...repo,
        summary,
      };
    }),
  );

  return reposWithSummary;
};

app.get("/trending", async (c) => {
  try {
    const cachedData = await c.env.GITHUB_TRENDING_JA.get<TrendingData>(
      KV_KEY,
      "json",
    );

    if (cachedData) {
      return c.json(cachedData);
    }

    return c.json(
      { error: "No data available. Please wait for the next update." },
      503,
    );
  } catch (error) {
    console.error("Error in /trending:", error);
    return c.json({ error: "Internal server error" }, 500);
  }
});

export const scheduled: ExportedHandlerScheduledHandler<Env> = async (
  _event,
  env,
  _ctx,
) => {
  try {
    const repos = await getTrendingReposWithSummary(env.API_KEY);
    const generatedAt = new Date().toISOString();

    const data: TrendingData = {
      items: repos,
      generatedAt,
    };

    await env.GITHUB_TRENDING_JA.put(KV_KEY, JSON.stringify(data));

    console.log(`Updated trending data in KV: ${repos.length} repositories`);
  } catch (error) {
    console.error("Error in scheduled handler:", error);
    throw error;
  }
};

const generateRSS = (data: TrendingData): string => {
  const { items, generatedAt } = data;
  const feed = new RSS({
    title: "GitHub Trending 日本語まとめ",
    description: "1日のGitHub Trendingを日本語で紹介",
    feed_url: "https://github-trending-ja.yashikota.com/feed",
    site_url: "https://github.com/yashikota/github-trending-ja",
    language: "ja",
    pubDate: new Date(generatedAt),
  });

  for (const repo of items) {
    feed.item({
      title: `${repo.title} - ${repo.summary}`,
      description: `
        ${repo.summary}
        <br><br>
        言語: ${repo.language || "不明"}<br>
        スター数: ${repo.stars} (+${repo.addStars})<br>
        フォーク数: ${repo.forks}
      `,
      url: repo.url,
      guid: `${repo.url}-${generatedAt}`,
      date: new Date(generatedAt),
    });
  }

  return feed.xml();
};

app.get("/feed", async (c) => {
  try {
    const cachedData = await c.env.GITHUB_TRENDING_JA.get<TrendingData>(
      KV_KEY,
      "json",
    );

    if (!cachedData) {
      return c.text("No data available", 503);
    }

    const rssXML = generateRSS(cachedData);
    return c.text(rssXML, 200, {
      "Content-Type": "application/rss+xml; charset=utf-8",
      "Cache-Control": "public, max-age=86400",
    });
  } catch (error) {
    console.error("Error in /feed:", error);
    return c.text("Internal server error", 500);
  }
});

export const getApp = (
  handler: (
    request: Request,
    env: Env,
    ctx: ExecutionContext,
  ) => Promise<Response>,
) => {
  app.all("*", async (context) => {
    return handler(
      context.req.raw,
      context.env,
      context.executionCtx as ExecutionContext,
    );
  });

  return app;
};
