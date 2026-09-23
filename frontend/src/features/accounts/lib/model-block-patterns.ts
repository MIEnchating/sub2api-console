// Keep UI filtering consistent with the backend's case-insensitive * / ? rules.
export function modelMatchesBlockPatterns(model: string, patterns: readonly string[]): boolean {
  const value = Array.from(model.trim().toLowerCase());
  return patterns.some((rawPattern) => {
    let previous: boolean[] = Array.from({ length: value.length + 1 }, (_, index) => index === 0);
    for (const token of rawPattern.trim().toLowerCase()) {
      const current: boolean[] = Array.from({ length: value.length + 1 }, () => false);
      current[0] = token === "*" && previous[0];
      for (let index = 1; index <= value.length; index++) {
        if (token === "*") current[index] = previous[index] || current[index - 1];
        else current[index] = previous[index - 1] && (token === "?" || token === value[index - 1]);
      }
      previous = current;
    }
    return previous[value.length];
  });
}
