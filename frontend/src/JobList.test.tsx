import { fireEvent, render, screen, within } from '@testing-library/react';
import { vi, test, expect } from 'vitest';
import { JobList } from './JobList';
import { JobView } from '../bindings/github.com/Mahmoud-s-Khedr/idm-enhanced/internal/desktop/models';
test('controls completed, queued, unknown-size and cancelled jobs with explicit retention', () => {
  const jobs = [
    new JobView({
      id: 'a',
      name: 'first',
      state: 'queued',
      total: -1,
      scheduled: '2030-01-01T12:00:00Z',
      retryAt: '2030-01-01T12:00:00Z',
      error: 'needs_attention',
      guidance: 'Preserve existing files; choose a new destination.',
    }),
    new JobView({ id: 'b', name: 'second', state: 'queued', total: 10 }),
    new JobView({
      id: 'c',
      name: 'complete',
      state: 'completed',
      total: 10,
      bytes: 10,
    }),
    new JobView({ id: 'd', name: 'cancelled', state: 'cancelled', total: 0 }),
  ];
  const control = vi.fn().mockResolvedValue(undefined);
  render(
    <JobList jobs={jobs} allJobs={jobs} running={false} control={control} />,
  );
  const first = within(screen.getByRole('article', { name: 'first' }));
  const second = within(screen.getByRole('article', { name: 'second' }));
  expect(first.getByRole('progressbar')).not.toHaveAttribute('value');
  expect(first.getByRole('progressbar')).toHaveAttribute(
    'aria-valuetext',
    '0.00 MiB downloaded; size unknown',
  );
  expect(first.getByText(/Preserve existing files/)).toBeInTheDocument();
  second.getByText('Move to front').click();
  expect(control).toHaveBeenLastCalledWith({
    action: 'reorder',
    order: ['b', 'a'],
  });
  for (const [label, action] of [
    ['Resume / retry', 'resume'],
    ['Pause', 'pause'],
    ['Clear schedule', 'schedule'],
  ] as const) {
    fireEvent.click(first.getByText(label));
    expect(control).toHaveBeenLastCalledWith(
      expect.objectContaining({ action, id: 'a' }),
    );
  }
  const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false);
  fireEvent.click(first.getByText('Restart'));
  confirm.mockReturnValue(true);
  fireEvent.click(first.getByText('Restart'));
  expect(control).toHaveBeenLastCalledWith({ action: 'restart', id: 'a' });
  confirm.mockRestore();
  fireEvent.click(first.getByText('Cancel…'));
  expect(first.getByText('Keep partial')).toHaveFocus();
  fireEvent.keyDown(first.getByText('Keep partial'), { key: 'Escape' });
  expect(first.getByText('Cancel…')).toHaveFocus();
  expect(first.queryByText('Keep partial')).not.toBeInTheDocument();
  fireEvent.click(first.getByText('Cancel…'));
  fireEvent.click(first.getByText('Go back'));
  expect(first.queryByText('Keep partial')).not.toBeInTheDocument();
  for (const [label, keepPartial] of [
    ['Keep partial', true],
    ['Discard partial', false],
  ] as const) {
    fireEvent.click(first.getByText('Cancel…'));
    fireEvent.click(first.getByText(label));
    expect(control).toHaveBeenLastCalledWith({
      action: 'cancel',
      id: 'a',
      keepPartial,
    });
  }
  const complete = within(screen.getByRole('article', { name: 'complete' }));
  for (const [label, action] of [
    ['Open file', 'open'],
    ['Show in folder', 'reveal'],
    ['Remove history', 'remove'],
  ] as const) {
    fireEvent.click(complete.getByText(label));
    expect(control).toHaveBeenLastCalledWith({ action, id: 'c' });
  }
  fireEvent.click(
    within(screen.getByRole('article', { name: 'cancelled' })).getByText(
      'Remove history',
    ),
  );
  expect(control).toHaveBeenLastCalledWith({ action: 'remove', id: 'd' });
  expect(
    within(screen.getByRole('article', { name: 'cancelled' })).getByRole(
      'progressbar',
    ),
  ).toHaveAttribute('value', '0');
});
test('empty lists explain how to proceed and active runs disable destructive actions', () => {
  const view = render(
    <JobList jobs={[]} allJobs={[]} running={false} control={vi.fn()} />,
  );
  expect(screen.getByText(/No downloads here/)).toBeInTheDocument();
  const jobs = [
    new JobView({ id: 'x', name: 'active', state: 'downloading', total: 100 }),
  ];
  view.rerender(
    <JobList jobs={jobs} allJobs={jobs} running={true} control={vi.fn()} />,
  );
  expect(screen.getByText('Cancel…')).toBeDisabled();
});

test('media progress counts complete objects independently from assembled file bytes', () => {
  const jobs = [
    new JobView({
      id: 'media',
      name: 'video.mp4',
      kind: 'hls',
      state: 'assembling',
      total: -1,
      mediaObjects: 2,
      mediaComplete: 1,
      mediaBytes: 8192,
      guidance: 'Complete segments are retained for assembly.',
    }),
  ];
  render(
    <JobList jobs={jobs} allJobs={jobs} running={true} control={vi.fn()} />,
  );
  expect(screen.getByRole('progressbar')).toHaveAttribute('value', '1');
  expect(screen.getByRole('progressbar')).toHaveAttribute('max', '2');
  expect(screen.getByRole('progressbar')).toHaveAttribute(
    'aria-valuetext',
    '1 of 2 segments; 0.01 MiB downloaded',
  );
  expect(
    screen.getByText('1 of 2 segments · 0.01 MiB downloaded'),
  ).toBeInTheDocument();
  expect(screen.queryByText(/size unknown/)).not.toBeInTheDocument();
});
