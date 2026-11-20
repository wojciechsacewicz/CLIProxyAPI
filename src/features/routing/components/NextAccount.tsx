import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { routingApi, type FiveHourPreview } from '@/services/api/routing';
import type { RoutingMember } from '@/types/routing';
import styles from '../RoutingPage.module.scss';

export function NextAccount({ model, members }: { model: string; members: RoutingMember[] }) {
  const { t, i18n } = useTranslation();
  const [result, setResult] = useState<FiveHourPreview | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    const refresh = async () => {
      try {
        const next = await routingApi.previewFiveHour(model, controller.signal);
        if (!controller.signal.aborted) setResult(next);
      } catch {
        if (!controller.signal.aborted) setResult({ reason: 'unsupported' });
      }
      if (!controller.signal.aborted) timer = setTimeout(() => void refresh(), 5000);
    };
    // The existing model field also accepts typed input.
    timer = setTimeout(() => void refresh(), 300);
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [model]);

  const member = result?.account
    ? members.find(
        (entry) => entry.location.kind === 'credential' && entry.location.name === result.account
      )
    : undefined;
  const message = result?.account
    ? (member?.name ?? result.account)
    : t(
        result?.reason === 'tie'
          ? 'routing.next_account_tie'
          : result?.reason === 'quota-unavailable' || result?.reason === 'mixed-providers'
            ? 'routing.next_account_round_robin'
            : result
              ? 'routing.next_account_unavailable'
              : 'routing.next_account_checking'
      );
  const remaining = result?.remaining_percent;
  return (
    <span className={styles.nextAccount} role="status">
      {t('routing.next_account_label')} <strong>{message}</strong>
      {typeof remaining === 'number'
        ? ' · ' +
          t('routing.next_account_remaining', {
            percent: new Intl.NumberFormat(i18n.resolvedLanguage, {
              style: 'percent',
              maximumFractionDigits: 1,
            }).format(remaining / 100),
          })
        : null}
    </span>
  );
}
