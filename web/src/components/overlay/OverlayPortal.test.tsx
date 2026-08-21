import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { useRef, useState } from 'react';
import { describe, expect, it } from 'vitest';
import { AnchoredOverlay, ModalOverlay } from './OverlayPortal';

function PopoverHarness() {
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  return <div>
    <button ref={trigger} onClick={() => setOpen(true)}>Open catalogue menu</button>
    {open && <AnchoredOverlay anchorRef={trigger} className="test-menu" role="menu" onDismiss={() => setOpen(false)}><button>Play</button></AnchoredOverlay>}
  </div>;
}

function ModalHarness() {
  const [open, setOpen] = useState(false);
  return <div>
    <button onClick={() => setOpen(true)}>Edit title</button>
    {open && <ModalOverlay labelledBy="test-title" className="test-dialog" onDismiss={() => setOpen(false)}><h1 id="test-title">Edit metadata</h1><button>Save</button></ModalOverlay>}
  </div>;
}

function DialogPopoverHarness() {
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  return <div>
    <button ref={trigger} onClick={() => setOpen(true)}>Open player settings</button>
    {open && <AnchoredOverlay anchorRef={trigger} ariaLabel="Player settings" className="test-dialog-popover" role="dialog" onDismiss={() => setOpen(false)}>
      <button>First setting</button>
      <button>Last setting</button>
    </AnchoredOverlay>}
  </div>;
}

function ComboboxPopoverHarness() {
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLInputElement>(null);
  return <div>
    <input ref={trigger} aria-label="Find media" onFocus={() => setOpen(true)} />
    {open && <AnchoredOverlay anchorRef={trigger} returnFocusRef={trigger} className="test-listbox" role="listbox" autoFocusComposite={false} onDismiss={() => setOpen(false)}>
      <a href="/media/one" role="option" aria-selected="false" tabIndex={-1}>First result</a>
    </AnchoredOverlay>}
  </div>;
}

function ComboboxDialogHarness() {
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLInputElement>(null);
  return <div>
    <input ref={trigger} role="combobox" aria-label="Search library" aria-haspopup="dialog" aria-expanded={open} onFocus={() => setOpen(true)} />
    {open && <AnchoredOverlay anchorRef={trigger} returnFocusRef={trigger} ariaLabel="Search suggestions" className="test-search-dialog" role="dialog" autoFocusComposite={false} onDismiss={() => setOpen(false)}>
      <a href="/media/one">First result</a>
      <button type="button">View all</button>
    </AnchoredOverlay>}
  </div>;
}

function NestedModalHarness() {
  const [outerOpen, setOuterOpen] = useState(false);
  const [innerOpen, setInnerOpen] = useState(false);
  return <div>
    <button onClick={() => setOuterOpen(true)}>Open outer dialog</button>
    {outerOpen && <ModalOverlay labelledBy="outer-title" className="outer-dialog" onDismiss={() => setOuterOpen(false)}>
      <h1 id="outer-title">Outer dialog</h1>
      <button onClick={() => setInnerOpen(true)}>Open inner dialog</button>
      <button>Save outer</button>
    </ModalOverlay>}
    {innerOpen && <ModalOverlay labelledBy="inner-title" className="inner-dialog" onDismiss={() => setInnerOpen(false)}>
      <h1 id="inner-title">Inner dialog</h1>
      <button>Save inner</button>
    </ModalOverlay>}
  </div>;
}

describe('overlay primitives', () => {
  it('renders anchored content in the shared portal and restores focus on Escape', async () => {
    render(<PopoverHarness />);
    const trigger = screen.getByRole('button', { name: 'Open catalogue menu' });
    fireEvent.click(trigger);
    const menu = screen.getByRole('menu');
    expect(document.getElementById('portico-overlays')).toContainElement(menu);
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
    await waitFor(() => expect(trigger).toHaveFocus());
  });

  it('moves and contains focus inside an anchored dialog', async () => {
    render(<DialogPopoverHarness />);
    const trigger = screen.getByRole('button', { name: 'Open player settings' });
    trigger.focus();
    fireEvent.click(trigger);
    const dialog = screen.getByRole('dialog', { name: 'Player settings' });
    const first = screen.getByRole('button', { name: 'First setting' });
    const last = screen.getByRole('button', { name: 'Last setting' });
    expect(document.getElementById('portico-overlays')).toContainElement(dialog);
    await waitFor(() => expect(first).toHaveFocus());

    last.focus();
    fireEvent.keyDown(window, { key: 'Tab' });
    expect(first).toHaveFocus();
    fireEvent.keyDown(window, { key: 'Tab', shiftKey: true });
    expect(last).toHaveFocus();

    fireEvent.keyDown(window, { key: 'Escape' });
    await waitFor(() => expect(trigger).toHaveFocus());
  });

  it('keeps focus in the input when a combobox owns listbox selection', async () => {
    render(<ComboboxPopoverHarness />);
    const input = screen.getByRole('textbox', { name: 'Find media' });
    input.focus();
    expect(await screen.findByRole('listbox')).toBeInTheDocument();
    await waitFor(() => expect(input).toHaveFocus());
    fireEvent.keyDown(input, { key: 'ArrowDown' });
    expect(input).toHaveFocus();
  });

  it('keeps focus in a combobox that controls a dialog of ordinary commands', async () => {
    render(<ComboboxDialogHarness />);
    const input = screen.getByRole('combobox', { name: 'Search library' });
    input.focus();
    const dialog = await screen.findByRole('dialog', { name: 'Search suggestions' });
    expect(input).toHaveFocus();
    expect(dialog).toContainElement(screen.getByRole('link', { name: 'First result' }));
    expect(dialog).toContainElement(screen.getByRole('button', { name: 'View all' }));
    expect(screen.queryByRole('option')).not.toBeInTheDocument();
  });

  it('dismisses a modal on Escape and returns focus to its opener', async () => {
    render(<ModalHarness />);
    const trigger = screen.getByRole('button', { name: 'Edit title' });
    trigger.focus();
    fireEvent.click(trigger);
    expect(screen.getByRole('dialog', { name: 'Edit metadata' })).toBeInTheDocument();
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    await waitFor(() => expect(trigger).toHaveFocus());
  });

  it('locks body scrolling and makes the page inert until the modal closes', async () => {
    document.body.style.overflow = 'clip';
    render(<ModalHarness />);
    const trigger = screen.getByRole('button', { name: 'Edit title' });
    trigger.focus();
    fireEvent.click(trigger);

    expect(document.body.style.overflow).toBe('hidden');
    expect(trigger.closest('[inert]')).not.toBeNull();
    fireEvent.keyDown(window, { key: 'Escape' });

    await waitFor(() => expect(trigger).toHaveFocus());
    expect(document.body.style.overflow).toBe('clip');
    expect(trigger.closest('[inert]')).toBeNull();
    document.body.style.overflow = '';
  });

  it('keeps the outer modal locked while a nested modal owns focus and Escape', async () => {
    render(<NestedModalHarness />);
    const outerTrigger = screen.getByRole('button', { name: 'Open outer dialog' });
    outerTrigger.focus();
    fireEvent.click(outerTrigger);
    const innerTrigger = await screen.findByRole('button', { name: 'Open inner dialog' });
    fireEvent.click(innerTrigger);

    expect(screen.getByRole('dialog', { name: 'Inner dialog' })).toBeInTheDocument();
    expect(screen.getByRole('dialog', { name: 'Outer dialog', hidden: true }).closest('[inert]')).not.toBeNull();
    fireEvent.keyDown(window, { key: 'Escape' });

    expect(screen.queryByRole('dialog', { name: 'Inner dialog' })).not.toBeInTheDocument();
    expect(screen.getByRole('dialog', { name: 'Outer dialog' })).toBeInTheDocument();
    expect(document.body.style.overflow).toBe('hidden');
    await waitFor(() => expect(innerTrigger).toHaveFocus());

    fireEvent.keyDown(window, { key: 'Escape' });
    expect(screen.queryByRole('dialog', { name: 'Outer dialog' })).not.toBeInTheDocument();
    await waitFor(() => expect(outerTrigger).toHaveFocus());
    expect(document.body.style.overflow).toBe('');
    expect(outerTrigger.closest('[inert]')).toBeNull();
  });
});
