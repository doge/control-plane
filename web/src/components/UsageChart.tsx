import { useEffect, useRef, useState } from "react";

export type UsagePoint = { time: string; cpu: number; memory: number };

/** Plot recent CPU and memory usage with a keyboard-accessible tooltip. */
export function UsageChart({ data }: { data: UsagePoint[] }) {
  const svgRef = useRef<SVGSVGElement>(null);
  const [size, setSize] = useState({ width: 600, height: 180 });
  const [hovered, setHovered] = useState<{
    key: "cpu" | "memory";
    index: number;
  } | null>(null);
  useEffect(() => {
    const svg = svgRef.current;
    if (!svg) return;
    const observer = new ResizeObserver(([entry]) => {
      const { width, height } = entry.contentRect;
      if (width > 0 && height > 0) setSize({ width, height });
    });
    observer.observe(svg);
    return () => observer.disconnect();
  }, []);

  const values = data.flatMap((point) => [
    Math.min(100, Math.max(0, point.cpu)),
    Math.min(100, Math.max(0, point.memory)),
  ]);
  const domain = (() => {
    if (!values.length) return { min: 0, max: 1 };
    const minimum = Math.min(...values);
    const maximum = Math.max(...values);
    const spread = maximum - minimum;
    const padding = Math.max(spread * 0.15, maximum * 0.05, 1);
    return {
      min: Math.max(0, minimum - padding),
      max: Math.min(100, maximum + padding),
    };
  })();
  const coordinates = (index: number, value: number) => {
    const range = Math.max(1, domain.max - domain.min);
    const scaled = (Math.min(100, Math.max(0, value)) - domain.min) / range;
    return {
      x:
        data.length < 2
          ? size.width / 2
          : (index / (data.length - 1)) * size.width,
      y:
        size.height -
        22 -
        Math.min(1, Math.max(0, scaled)) * (size.height - 44),
    };
  };
  const series = (key: "cpu" | "memory") =>
    data.map((point, index) => ({
      ...coordinates(index, point[key]),
      value: point[key],
      time: point.time,
      index,
    }));
  const cpuPoints = series("cpu");
  const memoryPoints = series("memory");
  const hoveredPoint =
    hovered && data[hovered.index]
      ? {
          key: hovered.key,
          ...coordinates(hovered.index, data[hovered.index][hovered.key]),
          ...data[hovered.index],
        }
      : null;
  const tooltipX = hoveredPoint
    ? Math.max(4, Math.min(size.width - 120, hoveredPoint.x - 56))
    : 0;
  const tooltipY = hoveredPoint
    ? Math.max(3, Math.min(size.height - 40, hoveredPoint.y - 43))
    : 0;

  return (
    <svg
      ref={svgRef}
      className="usage-chart-svg"
      viewBox={`0 0 ${size.width} ${size.height}`}
      preserveAspectRatio="none"
      role="group"
      aria-label="CPU and memory usage history"
    >
      {[16, size.height / 2, size.height - 22].map((y) => (
        <line
          key={y}
          x1="0"
          x2={size.width}
          y1={y}
          y2={y}
          className="usage-chart-grid"
        />
      ))}
      <polyline
        points={cpuPoints.map(({ x, y }) => `${x},${y}`).join(" ")}
        className="usage-chart-cpu"
      />
      <polyline
        points={memoryPoints.map(({ x, y }) => `${x},${y}`).join(" ")}
        className="usage-chart-memory"
      />
      {(["cpu", "memory"] as const).map((key) =>
        (key === "cpu" ? cpuPoints : memoryPoints).map((point) => (
          <circle
            key={`${key}-${point.index}`}
            className={`usage-chart-point usage-chart-${key}-point`}
            cx={point.x}
            cy={point.y}
            r={hovered?.key === key && hovered.index === point.index ? 5 : 3}
            tabIndex={0}
            aria-label={`${key === "cpu" ? "CPU" : "Memory"}: ${point.value.toFixed(
              1,
            )}% at ${point.time}`}
            onPointerEnter={() => setHovered({ key, index: point.index })}
            onPointerLeave={() => setHovered(null)}
            onFocus={() => setHovered({ key, index: point.index })}
            onBlur={() => setHovered(null)}
          />
        )),
      )}
      {hoveredPoint && (
        <g className="usage-chart-tooltip" pointerEvents="none">
          <rect x={tooltipX} y={tooltipY} width="116" height="37" rx="6" />
          <text x={tooltipX + 8} y={tooltipY + 15}>
            {hoveredPoint.key === "cpu" ? "CPU" : "Memory"}:{" "}
            {hoveredPoint[hoveredPoint.key].toFixed(1)}%
          </text>
          <text
            className="usage-chart-tooltip-time"
            x={tooltipX + 8}
            y={tooltipY + 29}
          >
            {hoveredPoint.time}
          </text>
        </g>
      )}
    </svg>
  );
}
