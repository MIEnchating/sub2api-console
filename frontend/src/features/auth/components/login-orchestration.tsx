import { Activity, GitBranch, Layers3, SlidersHorizontal, type LucideIcon } from "lucide-react";
import { useId, type ReactElement } from "react";

type ManagementNodeProps = { x: number; y: number; label: string; icon: LucideIcon };

const illustrationLabel = "Console 统一管理 Sub2API 的账号分组、健康监测、倍率同步与自动调度";
const capabilities = [
  { label: "账号分组", icon: Layers3 },
  { label: "健康监测", icon: Activity },
  { label: "倍率同步", icon: SlidersHorizontal },
  { label: "自动调度", icon: GitBranch },
];

function ManagementNode(props: ManagementNodeProps): ReactElement {
  const Icon = props.icon;
  return (
    <g transform={`translate(${props.x} ${props.y})`}>
      <rect width="164" height="58" className="login-map-node" />
      <Icon x="18" y="18" width="22" height="22" strokeWidth="1.6" className="login-map-icon" />
      <text x="53" y="35" className="login-map-label">
        {props.label}
      </text>
    </g>
  );
}

/** A product illustration, not live service status or a traffic metric. */
export function LoginOrchestration(): ReactElement {
  const titleID = useId();
  return (
    <div data-slot="login-orchestration" className="login-map h-full w-full">
      <svg
        viewBox="0 0 680 380"
        role="img"
        aria-labelledby={titleID}
        className="hidden h-full w-full lg:block"
      >
        <title id={titleID}>{illustrationLabel}</title>
        <g aria-hidden="true">
          <g className="login-map-links" fill="none" strokeWidth="1.5">
            <path d="M184 94H217Q244 94 244 121V155H272" />
            <path d="M496 94H463Q436 94 436 121V155H408" />
            <path d="M184 244H217Q244 244 244 217V195H272" />
            <path d="M496 244H463Q436 244 436 217V195H408" />
            <path d="M340 223V275" />
          </g>
          <g className="login-map-signals" fill="none" strokeWidth="2.5" strokeLinecap="round">
            <path d="M272 155H244V121Q244 94 217 94H184" />
            <path d="M408 155H436V121Q436 94 463 94H496" />
            <path d="M272 195H244V217Q244 244 217 244H184" />
            <path d="M408 195H436V217Q436 244 463 244H496" />
            <path d="M340 223V275" />
          </g>
          <ManagementNode x={20} y={65} label="账号与分组" icon={Layers3} />
          <ManagementNode x={496} y={65} label="健康监测" icon={Activity} />
          <ManagementNode x={20} y={215} label="倍率同步" icon={SlidersHorizontal} />
          <ManagementNode x={496} y={215} label="自动调度" icon={GitBranch} />
          <rect x="272" y="127" width="136" height="96" className="login-map-center" />
          <image href="/console-mark.svg" x="319" y="143" width="42" height="42" />
          <text x="340" y="208" textAnchor="middle" className="login-map-console">
            Console
          </text>
          <rect x="264" y="275" width="152" height="54" className="login-map-service" />
          <text x="340" y="308" textAnchor="middle" className="login-map-service-name">
            Sub2API
          </text>
        </g>
      </svg>
      <div
        role="img"
        aria-label={illustrationLabel}
        className="flex h-full flex-col justify-center gap-6 lg:hidden"
      >
        <div aria-hidden="true" className="grid grid-cols-[1fr_40px_1fr] items-center">
          <div className="login-map-mobile-console flex items-center justify-center gap-2 rounded-lg border px-3 py-3">
            <img src="/console-mark.svg" alt="" width={24} height={24} />
            <span className="text-sm font-semibold">Console</span>
          </div>
          <svg viewBox="0 0 40 12" className="w-full" fill="none">
            <path d="M0 6H40" className="login-map-links" />
            <g className="login-map-signals">
              <path d="M0 6H40" strokeWidth="2" />
            </g>
            <path d="m34 2 4 4-4 4" className="login-map-links" />
          </svg>
          <div className="login-map-mobile-service rounded-lg border px-3 py-3 text-center text-sm font-semibold">
            Sub2API
          </div>
        </div>
        <div aria-hidden="true" className="grid grid-cols-4 gap-1">
          {capabilities.map((capability) => (
            <div
              key={capability.label}
              className="flex flex-col items-center gap-2 text-xs text-muted-foreground"
            >
              <capability.icon className="login-map-icon size-4" strokeWidth={1.5} />
              <span>{capability.label}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
