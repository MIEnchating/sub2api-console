import jsep from "jsep";
import type { BinaryExpression, CallExpression, Expression } from "jsep";

import { formatModelPriceNumber } from "./pricing-number";

const priceVariables = new Set(["p", "c", "cr", "cc", "cc1h", "img", "img_o", "ai", "ao"]);

type PriceTerm = { variable: string; price: string };

function priceTerms(node: Expression): PriceTerm[] | null {
  if (node.type !== "BinaryExpression") return null;
  const binary = node as BinaryExpression;
  if (binary.operator === "+") {
    const left = priceTerms(binary.left);
    const right = priceTerms(binary.right);
    return left && right ? [...left, ...right] : null;
  }
  if (
    binary.operator !== "*" ||
    binary.left.type !== "Identifier" ||
    typeof binary.left.name !== "string" ||
    !priceVariables.has(binary.left.name) ||
    binary.right.type !== "Literal" ||
    typeof binary.right.value !== "number" ||
    !Number.isFinite(binary.right.value) ||
    binary.right.value < 0
  ) {
    return null;
  }
  return [{ variable: binary.left.name, price: formatModelPriceNumber(binary.right.value) }];
}

function canonicalNode(node: Expression): unknown {
  if (node.type === "Literal") {
    // Preserve types, string contents and exact non-price condition boundaries.
    return { type: node.type, raw: node.raw };
  }
  if (node.type === "CallExpression") {
    const call = node as CallExpression;
    if (
      call.callee.type === "Identifier" &&
      call.callee.name === "tier" &&
      call.arguments.length === 2
    ) {
      const terms = priceTerms(call.arguments[1]);
      // Only reorder a complete sum of unique price terms; retain all other syntax.
      if (terms && new Set(terms.map((term) => term.variable)).size === terms.length) {
        terms.sort((left, right) => left.variable.localeCompare(right.variable));
        return {
          type: "PriceTier",
          label: canonicalNode(call.arguments[0]),
          terms,
        };
      }
    }
  }
  return Object.fromEntries(
    Object.entries(node).map(([key, value]) => [key, canonicalValue(value)]),
  );
}

function canonicalValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(canonicalValue);
  if (value && typeof value === "object" && "type" in value && typeof value.type === "string") {
    return canonicalNode(value as Expression);
  }
  return value;
}

export function billingExpressionsEqual(left: string, right: string): boolean {
  if (left.trim() === right.trim()) return true;
  if (!left.trim() || !right.trim()) return false;
  try {
    return JSON.stringify(canonicalNode(jsep(left))) === JSON.stringify(canonicalNode(jsep(right)));
  } catch {
    // Unsupported syntax remains a visible difference; never evaluate billing code.
    return false;
  }
}
