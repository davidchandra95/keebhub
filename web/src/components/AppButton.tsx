import {Button, type ButtonProps} from '@astryxdesign/core/Button';

type AppButtonProps = Pick<
  ButtonProps,
  'label' | 'onClick' | 'type' | 'isDisabled' | 'isLoading'
>;

// Keep beta design-system details inside this small adapter.
export function AppButton(props: AppButtonProps) {
  return <Button variant="primary" {...props} />;
}
