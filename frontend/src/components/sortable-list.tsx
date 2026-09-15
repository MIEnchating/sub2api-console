import {
  closestCorners,
  pointerWithin,
  DndContext,
  DragOverlay,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
  type CollisionDetection,
  type DropAnimation,
} from "@dnd-kit/core";
import {
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { useReducedMotion } from "motion/react";
import { useId, useState, type CSSProperties, type ReactNode } from "react";
import { GripVertical } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { createPortal } from "react-dom";

type SortableIdentity = { id: string; label: string };

const sortingDuration = 200;
const sortingEasing = "cubic-bezier(0.22, 1, 0.36, 1)";
const dropAnimation: DropAnimation = {
  duration: sortingDuration,
  easing: sortingEasing,
  // The compact overlay fades into the full row; keep that row visible throughout.
  sideEffects: null,
  keyframes: ({ transform }) => [
    { transform: CSS.Transform.toString(transform.initial), opacity: 1 },
    { transform: CSS.Transform.toString(transform.final), opacity: 0 },
  ],
};

const sortableCollisionDetection: CollisionDetection = (args) => {
  const pointer = args.pointerCoordinates;
  if (!pointer) return closestCorners(args);
  const hits = pointerWithin(args);
  if (hits.length) return hits;
  const rects = [...args.droppableRects.values()];
  // Keep gaps between rows droppable while cancelling drops outside the list.
  if (
    rects.length &&
    pointer.x >= Math.min(...rects.map((rect) => rect.left)) &&
    pointer.x <= Math.max(...rects.map((rect) => rect.right)) &&
    pointer.y >= Math.min(...rects.map((rect) => rect.top)) &&
    pointer.y <= Math.max(...rects.map((rect) => rect.bottom))
  ) {
    return closestCorners(args);
  }
  return [];
};

export function SortableList(props: {
  items: SortableIdentity[];
  disabled?: boolean;
  onMove: (from: number, to: number) => void;
  children: ReactNode;
}) {
  const id = useId();
  const [active, setActive] = useState<string | null>(null);
  const reducedMotion = useReducedMotion();
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
      scrollBehavior: "auto",
    }),
  );
  function finish(event: DragEndEvent): void {
    setActive(null);
    if (props.disabled || !event.over || event.active.id === event.over.id) return;
    const from = props.items.findIndex((item) => item.id === event.active.id);
    const to = props.items.findIndex((item) => item.id === event.over?.id);
    if (from >= 0 && to >= 0) props.onMove(from, to);
  }
  return (
    <DndContext
      id={id}
      sensors={sensors}
      collisionDetection={sortableCollisionDetection}
      onDragStart={(event) => setActive(String(event.active.id))}
      onDragEnd={finish}
      onDragCancel={() => setActive(null)}
      accessibility={{
        screenReaderInstructions: {
          draggable: "按空格开始拖动，方向键移动，空格放下，Escape 取消。",
        },
        announcements: {
          onDragStart: () => "已开始拖动",
          onDragOver: ({ over }) =>
            over
              ? `目标位置 ${props.items.findIndex((item) => item.id === over.id) + 1}`
              : "已移出列表",
          onDragEnd: () => "拖动已结束",
          onDragCancel: () => "已取消拖动",
        },
      }}
    >
      <SortableContext
        items={props.items.map((item) => item.id)}
        strategy={verticalListSortingStrategy}
      >
        {props.children}
      </SortableContext>
      {typeof document !== "undefined"
        ? createPortal(
            <DragOverlay
              className="pointer-events-none"
              transition={reducedMotion ? "none" : undefined}
              dropAnimation={reducedMotion ? null : dropAnimation}
            >
              {active && !props.disabled ? (
                <div className="flex max-w-80 items-center gap-2 rounded-md border border-primary bg-background px-3 py-2 text-sm shadow-lg">
                  <GripVertical aria-hidden="true" />
                  <span className="truncate">
                    {props.items.find((item) => item.id === active)?.label}
                  </span>
                </div>
              ) : null}
            </DragOverlay>,
            document.body,
          )
        : null}
    </DndContext>
  );
}

export function SortableItem(props: {
  id: string;
  label: string;
  disabled?: boolean;
  children: (item: {
    ref: (node: HTMLElement | null) => void;
    style: CSSProperties;
    handle: ReactNode;
    dragging: boolean;
    over: boolean;
  }) => ReactNode;
}) {
  const reducedMotion = useReducedMotion();
  const sortable = useSortable({
    id: props.id,
    disabled: props.disabled,
    transition: { duration: reducedMotion ? 0 : sortingDuration, easing: sortingEasing },
  });
  return props.children({
    ref: sortable.setNodeRef,
    style: {
      // Entry animations restart when rows move and override the drag opacity.
      animation: "none",
      transform: CSS.Transform.toString(sortable.transform),
      transition: reducedMotion
        ? sortable.transition
        : [sortable.transition, `opacity ${sortingDuration}ms ${sortingEasing}`]
            .filter(Boolean)
            .join(", "),
      opacity: sortable.isDragging ? 0.4 : 1,
    },
    dragging: sortable.isDragging,
    over: sortable.isOver,
    handle: (
      <Tooltip disabled={sortable.isDragging}>
        <TooltipTrigger
          render={
            <Button
              ref={sortable.setActivatorNodeRef}
              type="button"
              size="icon"
              variant="ghost"
              className="touch-none shrink-0 cursor-grab active:cursor-grabbing"
              disabled={props.disabled}
              {...sortable.attributes}
              {...sortable.listeners}
              aria-pressed={sortable.isDragging}
              aria-label={`拖动${props.label}`}
            />
          }
        >
          <GripVertical aria-hidden="true" />
        </TooltipTrigger>
        <TooltipContent>{`拖动${props.label}`}</TooltipContent>
      </Tooltip>
    ),
  });
}
