import type { CSSProperties } from "react";

/** Show the full node capacity while restricting selection to currently free units. */
export function ResourceSlider({
  label,
  value,
  minimum,
  total,
  available,
  capacityKnown,
  format,
  onChange,
}: {
  label: string;
  value: number;
  minimum: number;
  total: number;
  available: number;
  capacityKnown: boolean;
  format: (units: number) => string;
  onChange: (units: number) => void;
}) {
  const maxSelectable = Math.min(total, available);
  const disabled = !capacityKnown || total < minimum || maxSelectable < minimum;
  const safeValue = disabled
    ? minimum
    : Math.min(Math.max(value, minimum), maxSelectable);
  const range = total - minimum;
  const selectedPercent =
    disabled
      ? 0
      : range > 0
      ? Math.min(Math.max(((safeValue - minimum) / range) * 100, 0), 100)
      : 100;
  const availablePercent =
    disabled
      ? 0
      : range > 0
      ? Math.min(Math.max(((maxSelectable - minimum) / range) * 100, 0), 100)
      : maxSelectable >= minimum
        ? 100
        : 0;
  const style = {
    "--resource-track": `linear-gradient(90deg, var(--accent) 0%, var(--accent) ${selectedPercent}%, color-mix(in srgb, var(--accent) 24%, var(--surface-raised)) ${selectedPercent}%, color-mix(in srgb, var(--accent) 24%, var(--surface-raised)) ${availablePercent}%, color-mix(in srgb, var(--muted) 14%, var(--surface)) ${availablePercent}%, color-mix(in srgb, var(--muted) 14%, var(--surface)) 100%)`,
  } as CSSProperties;

  return (
    <div className={`resource-slider${disabled ? " is-unavailable" : ""}`}>
      <div className="resource-slider-heading">
        <span>{label}</span>
        <strong>{disabled ? "Unavailable" : format(safeValue)}</strong>
      </div>
      <input
        aria-label={label}
        aria-valuemax={Math.max(minimum, maxSelectable)}
        aria-valuemin={minimum}
        className="resource-slider-input"
        disabled={disabled}
        max={Math.max(total, minimum)}
        min={minimum}
        onChange={(event) => {
          const next = Number(event.target.value);
          onChange(Math.min(Math.max(next, minimum), maxSelectable));
        }}
        role="slider"
        style={style}
        step={1}
        type="range"
        value={safeValue}
      />
      <div className="resource-slider-scale">
        <span>{format(minimum)} minimum</span>
        <span>{capacityKnown ? `${format(total)} total` : "Waiting for node"}</span>
      </div>
      <div className="resource-slider-availability">
        {!capacityKnown
          ? "Waiting for node capacity"
          : disabled
          ? "No unallocated capacity for this resource"
          : `Up to ${format(maxSelectable)} can be allocated · ${format(Math.max(total - maxSelectable, 0))} unavailable`}
      </div>
    </div>
  );
}
