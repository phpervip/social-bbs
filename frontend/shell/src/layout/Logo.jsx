import React from 'react';

/**
 * Logo — pure-CSS "B": 48px rounded square,
 * linear-gradient(135deg, #c19a6b, #8b5a2b), white bold letter B.
 */
export default function Logo({ size = 48 }) {
  return (
    <span
      className="b-logo"
      style={{
        width: size,
        height: size,
        fontSize: size * 0.58,
        lineHeight: `${size}px`,
        borderRadius: size * 0.25,
        background: 'linear-gradient(135deg, #c19a6b, #8b5a2b)',
      }}
      aria-label="B"
    >
      B
    </span>
  );
}
