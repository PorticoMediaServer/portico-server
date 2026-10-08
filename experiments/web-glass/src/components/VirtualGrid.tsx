import { useLayoutEffect, useRef, useState, type ReactNode } from 'react';
import { useWindowVirtualizer } from '@tanstack/react-virtual';

/**
 * Window-scrolled virtual grid. Only the rows in (and just around) the viewport are
 * mounted, so a 500,000-item library costs the same to render as a 50-item one.
 */
export function VirtualGrid<T>({
  items,
  minWidth,
  gap = 20,
  rowGap,
  aspect,
  textHeight = 52,
  render,
  getKey,
  overscan = 4,
}: {
  items: T[];
  minWidth: number;
  gap?: number;
  rowGap?: number;
  aspect: number; // width / height of the artwork
  textHeight?: number;
  render: (item: T, index: number) => ReactNode;
  getKey: (item: T, index: number) => string;
  overscan?: number;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);
  const [offset, setOffset] = useState(0);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const measure = () => {
      setWidth(el.clientWidth);
      setOffset(el.getBoundingClientRect().top + window.scrollY);
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  const cols = Math.max(1, Math.floor((width + gap) / (minWidth + gap)));
  const colW = width ? (width - gap * (cols - 1)) / cols : minWidth;
  const rg = rowGap ?? gap + 4;
  const rowH = colW / aspect + textHeight + rg;
  const rows = Math.ceil(items.length / cols);
  const v = useWindowVirtualizer({ count: rows, estimateSize: () => rowH, overscan, scrollMargin: offset });
  useLayoutEffect(() => {
    v.measure();
  }, [rowH, cols, v]);

  return (
    <div ref={ref} className="vgrid" style={{ height: v.getTotalSize(), position: 'relative' }}>
      {width > 0 &&
        v.getVirtualItems().map((row) => (
          <div
            key={row.key}
            className="vgrid-row"
            style={{
              position: 'absolute',
              top: 0,
              left: 0,
              right: 0,
              transform: `translateY(${row.start - v.options.scrollMargin}px)`,
              display: 'grid',
              gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`,
              columnGap: gap,
            }}
          >
            {items.slice(row.index * cols, row.index * cols + cols).map((it, j) => (
              <div key={getKey(it, row.index * cols + j)} className="vgrid-cell">
                {render(it, row.index * cols + j)}
              </div>
            ))}
          </div>
        ))}
    </div>
  );
}
