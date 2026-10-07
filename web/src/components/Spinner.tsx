/** Show an accessible loading indicator that keeps a square footprint. */
export function Spinner({ label }: { label?: string }) {
  return (
    <span
      className="spinner"
      role={label ? "status" : undefined}
      aria-label={label}
      aria-hidden={label ? undefined : true}
    />
  );
}
