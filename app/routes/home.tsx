import { Calendar, GitFork, Loader2, Rss, Star } from "lucide-react";
import { useEffect, useState } from "react";
import type { TrendingRepoWithSummary } from "server";

export function meta() {
  return [
    { title: "GitHub Trending 日本語まとめ" },
    { name: "description", content: "1日の GitHub Trending を日本語で紹介" },
  ];
}

export default function Home() {
  const [repos, setRepos] = useState<TrendingRepoWithSummary[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [generatedAt, setGeneratedAt] = useState<string | null>(null);

  const formatDate = (dateString: string) => {
    const date = new Date(dateString);
    const options: Intl.DateTimeFormatOptions = {
      year: "numeric",
      month: "long",
      day: "numeric",
      timeZone: "Asia/Tokyo",
    };
    return date.toLocaleDateString("ja-JP", options);
  };

  useEffect(() => {
    fetch("/trending")
      .then((res) => {
        if (!res.ok) throw new Error("Failed to fetch");
        return res.json();
      })
      .then((data) => {
        const typedData = data as {
          items?: TrendingRepoWithSummary[];
          generatedAt?: string;
        };
        if (typedData?.items && Array.isArray(typedData.items)) {
          setRepos(typedData.items);
          setGeneratedAt(typedData.generatedAt || null);
        } else {
          setError("Invalid data format");
        }
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  if (loading)
    return (
      <div className="flex items-center justify-center min-h-[50vh]">
        <div className="flex flex-col items-center gap-4">
          <Loader2 className="w-8 h-8 animate-spin text-blue-500" />
          <p className="text-gray-600 dark:text-gray-400">読み込み中...</p>
        </div>
      </div>
    );
  if (error) return <div className="text-red-500">Error: {error}</div>;
  if (!repos) return null;

  return (
    <div className="max-w-2xl mx-auto p-4">
      <div className="flex items-center justify-between mb-4">
        <h1 className="text-2xl font-bold">GitHub Trending 日本語まとめ</h1>
        <a
          href="/feed"
          className="text-orange-500 hover:underline flex items-center gap-1"
        >
          <Rss size={16} />
          RSS
        </a>
      </div>
      <p className="text-gray-600 dark:text-gray-400 mb-8">
        1日の
        <a
          href="https://github.com/trending"
          target="_blank"
          rel="noopener noreferrer"
          className="text-blue-600 hover:underline mx-1"
        >
          GitHub Trending
        </a>
        を日本語で紹介
      </p>

      {generatedAt && (
        <div className="text-center mb-8">
          <p className="text-xl font-semibold text-gray-700 dark:text-gray-300 flex items-center justify-center gap-2">
            <Calendar size={20} />
            {formatDate(generatedAt)}のトレンド
          </p>
        </div>
      )}

      <ul className="space-y-4">
        {repos.map((repo) => (
          <li
            key={repo.title}
            className="border rounded-lg bg-white dark:bg-gray-900 transition hover:shadow-lg"
          >
            <a
              href={repo.url}
              target="_blank"
              rel="noopener noreferrer"
              className="block p-4 text-current no-underline"
            >
              <div className="text-lg font-semibold text-blue-600">
                {repo.title}
              </div>
              <p className="mt-1 text-gray-700 dark:text-gray-300">
                {repo.summary}
              </p>
              <div className="flex flex-wrap gap-4 mt-2 text-sm text-gray-500 dark:text-gray-400">
                {repo.language && (
                  <span>
                    <span
                      className="inline-block w-3 h-3 rounded-full mr-1 align-middle"
                      style={{ backgroundColor: repo.languageColor || "#ccc" }}
                    />
                    {repo.language}
                  </span>
                )}
                <span className="flex items-center gap-1">
                  <Star size={16} /> {repo.stars}（+{repo.addStars}）
                </span>
                <span className="flex items-center gap-1">
                  <GitFork size={16} /> {repo.forks}
                </span>
              </div>
            </a>
          </li>
        ))}
      </ul>
    </div>
  );
}
