import {Card} from '@astryxdesign/core/Card';
import type {PropsWithChildren} from 'react';

// Keep beta design-system details inside this small adapter.
export function AppCard({children}: PropsWithChildren) {
  return (
    <Card className="status-card" padding={6} elevation="low">
      {children}
    </Card>
  );
}
