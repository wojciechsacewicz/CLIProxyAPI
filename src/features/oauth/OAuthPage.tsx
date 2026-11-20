import { useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { useRevealGroup } from '@/hooks/motion';
import { useAuthStore, useThemeStore } from '@/stores';
import { OAuthHeader } from './components/OAuthHeader';
import { LoginProxyField } from './components/LoginProxyField';
import { ProviderTile } from './components/ProviderTile';
import { OAuthFlowDialog, type DialogHeading } from './components/OAuthFlowDialog';
import {
  useOAuthFlows,
  usePluginOAuthProviders,
  type ProviderFlowState,
} from './hooks/useOAuthFlows';
import {
  OAUTH_PROVIDERS,
  resolveThemedIcon,
  supportsManualCallback,
  type OAuthProviderCard,
} from './providers';
import styles from './OAuthPage.module.scss';

const VISIBLE_OAUTH_PROVIDERS = new Set(['codex', 'anthropic', 'antigravity', 'xai']);

/**
 * OAuth 登录：提供商画廊（一屏看全）+ 聚焦的授权对话框。
 *
 * 点磁贴即开始登录；对话框可随时关闭，授权在后台继续，磁贴副行实时反映进度。
 * 对话框关闭后保留最后一个目标，让退出动画期间内容不消失。
 */
export function OAuthPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const apiBase = useAuthStore((state) => state.apiBase);
  const resolvedTheme = useThemeStore((state) => state.resolvedTheme);
  const revealRef = useRevealGroup<HTMLDivElement>();

  const pluginProviders = usePluginOAuthProviders(apiBase);
  const providerCards = useMemo<OAuthProviderCard[]>(
    () => [
      ...OAUTH_PROVIDERS.filter((card) => VISIBLE_OAUTH_PROVIDERS.has(card.id)),
      ...pluginProviders,
    ],
    [pluginProviders]
  );

  const focusedRef = useRef<string | null>(null);
  const [dialog, setDialog] = useState<string | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const flows = useOAuthFlows({
    providerCards,
    isFocused: (provider) => focusedRef.current === provider,
  });
  const labelOf = (card: OAuthProviderCard) =>
    card.kind === 'plugin'
      ? card.title
      : typeof card.label === 'string'
        ? card.label
        : t(card.label.key);

  const captionOf = (card: OAuthProviderCard) => {
    if (card.kind === 'plugin') return t('auth_login.method_plugin');
    const method = t(
      card.flow === 'device' ? 'auth_login.method_device' : 'auth_login.method_browser'
    );
    return card.domain ? `${card.domain} · ${method}` : method;
  };

  const iconOf = (card: OAuthProviderCard) =>
    card.kind === 'plugin' ? card.icon : resolveThemedIcon(card.icon, resolvedTheme);

  const headingOf = (card: OAuthProviderCard): DialogHeading => ({
    title:
      card.kind === 'plugin'
        ? t('auth_login.plugin_oauth_title', { name: card.title })
        : t(card.titleKey),
    caption: captionOf(card),
    icon: iconOf(card),
  });

  const signInNoteOf = (card: OAuthProviderCard) =>
    card.kind === 'builtin' && card.signInNoteKey ? t(card.signInNoteKey) : undefined;

  const tileStatusLabel = (status: ProviderFlowState['status']) =>
    status === 'waiting'
      ? t('auth_login.tile_waiting')
      : status === 'success'
        ? t('auth_login.tile_success')
        : status === 'error'
          ? t('auth_login.tile_error')
          : undefined;

  const openProvider = (card: OAuthProviderCard) => {
    focusedRef.current = card.id;
    setDialog(card.id);
    setDialogOpen(true);
    // 已有进行中/失败/刚成功的尝试 → 只是重新打开查看；否则立即开始登录
    if (!flows.states[card.id]?.status) void flows.startAuth(card.id);
  };

  const closeDialog = () => {
    if (dialog) flows.dismissSuccess(dialog);
    focusedRef.current = null;
    setDialogOpen(false);
  };

  const viewAuthFiles = () => navigate('/auth-files');

  const dialogCard = providerCards.find((card) => card.id === dialog);
  const providerCount = providerCards.length;
  const renderTile = (card: OAuthProviderCard, index: number) => {
    const status = flows.states[card.id]?.status;
    return (
      <ProviderTile
        key={card.id}
        label={labelOf(card)}
        caption={captionOf(card)}
        icon={iconOf(card)}
        index={index}
        status={status}
        statusLabel={tileStatusLabel(status)}
        onOpen={() => openProvider(card)}
      />
    );
  };
  const waitingCount = Object.values(flows.states).filter(
    (state) => state.status === 'waiting'
  ).length;

  return (
    <div className={styles.page} ref={revealRef}>
      <OAuthHeader
        providerCount={providerCount}
        waitingCount={waitingCount}
        addedCount={flows.addedCount}
      />

      {/* 可选登录代理：在点磁贴之前设好，不改变一键登录的快路径 */}
      <LoginProxyField
        value={flows.loginProxy}
        rejectedValue={flows.loginProxyRejected}
        onChange={flows.setLoginProxy}
      />

      <div className={styles.grid}>{providerCards.map(renderTile)}</div>

      {dialogCard && (
        <OAuthFlowDialog
          open={dialogOpen}
          providerId={dialogCard.id}
          heading={headingOf(dialogCard)}
          state={flows.states[dialogCard.id] ?? {}}
          text={(suffix) => flows.providerText(dialogCard.id, suffix)}
          supportsCallback={supportsManualCallback(dialogCard)}
          signInNote={signInNoteOf(dialogCard)}
          onStart={() => void flows.startAuth(dialogCard.id)}
          onCancel={() => void flows.cancelAuth(dialogCard.id)}
          onCallbackChange={(value) => flows.setCallbackInput(dialogCard.id, value)}
          onCallbackSubmit={() => void flows.submitCallback(dialogCard.id)}
          onViewAuthFiles={viewAuthFiles}
          onClose={closeDialog}
        />
      )}
    </div>
  );
}
