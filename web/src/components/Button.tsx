import type { AnchorHTMLAttributes, ButtonHTMLAttributes } from "react";

type Variant = "primary" | "subtle" | "danger" | "danger-outline";
type Size = "default" | "small" | "tiny";

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: Variant;
  size?: Size;
  fullWidth?: boolean;
};

type ButtonLinkProps = AnchorHTMLAttributes<HTMLAnchorElement> & {
  variant?: Variant;
  size?: Size;
  fullWidth?: boolean;
};

const variants: Record<Variant, string> = {
  primary: "primary",
  subtle: "subtle",
  danger: "danger",
  "danger-outline": "danger-outline",
};
const sizes: Record<Size, string> = {
  default: "",
  small: "small",
  tiny: "tiny",
};

/** Render a consistently styled action button with native button behavior. */
export function Button({
  variant = "subtle",
  size = "default",
  fullWidth,
  className = "",
  ...props
}: ButtonProps) {
  const classes =
    `button ${variants[variant]} ${sizes[size]} ${fullWidth ? "full-width" : ""} ${className}`.trim();
  return <button {...props} className={classes} />;
}

/** Render a link styled as an action button. */
export function ButtonLink({
  variant = "subtle",
  size = "default",
  fullWidth,
  className = "",
  ...props
}: ButtonLinkProps) {
  const classes =
    `button ${variants[variant]} ${sizes[size]} ${fullWidth ? "full-width" : ""} ${className}`.trim();
  return <a {...props} className={classes} />;
}
