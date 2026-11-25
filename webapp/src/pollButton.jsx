import React, {useCallback, useEffect, useState} from 'react';

// Mattermost отдаёт плагинам store в initialize(); через него читаем текущий
// канал и диспатчим открытие окна. Прятать это в контекст React не нужно.
let pluginStore = null;

// Внутреннее действие клиента Mattermost: по нему открывается диалог, который
// плагин отправил серверной функцией OpenInteractiveDialog.
const RECEIVED_DIALOG_TRIGGER_ID = 'RECEIVED_DIALOG_TRIGGER_ID';

const STYLE_ID = 'polls-plugin-toolbar-styles';

const STYLES = `
.polls-toolbar-button {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    padding: 0;
    border: none;
    border-radius: 4px;
    background: transparent;
    color: rgba(var(--center-channel-color-rgb), 0.72);
    cursor: pointer;
    transition: background-color 0.15s ease, color 0.15s ease;
}
.polls-toolbar-button:hover:not(:disabled) {
    background: rgba(var(--center-channel-color-rgb), 0.08);
    color: rgba(var(--center-channel-color-rgb), 0.88);
}
.polls-toolbar-button:active:not(:disabled) {
    background: rgba(var(--center-channel-color-rgb), 0.16);
}
.polls-toolbar-button:disabled {
    color: rgba(var(--center-channel-color-rgb), 0.32);
    cursor: default;
}
.polls-toolbar-button:focus-visible {
    outline: 2px solid var(--button-bg);
    outline-offset: 1px;
}
`;

const injectStyles = () => {
    if (document.getElementById(STYLE_ID)) {
        return;
    }
    const style = document.createElement('style');
    style.id = STYLE_ID;
    style.textContent = STYLES;
    document.head.appendChild(style);
};

const randomTriggerId = () => {
    const alphabet = 'abcdefghijklmnopqrstuvwxyz0123456789';
    let out = 't';
    for (let i = 0; i < 25; i++) {
        out += alphabet[Math.floor(Math.random() * alphabet.length)];
    }
    return out;
};

const currentChannelId = () => {
    try {
        return pluginStore.getState().entities.channels.currentChannelId || '';
    } catch (error) {
        return '';
    }
};

const currentTeamId = () => {
    try {
        return pluginStore.getState().entities.teams.currentTeamId || '';
    } catch (error) {
        return '';
    }
};

// Столбики диаграммы, а не список: в этой панели уже есть две иконки списков,
// и «точки с линиями» читались бы как третья.
const PollIcon = () => (
    <svg
        width='18'
        height='18'
        viewBox='0 0 24 24'
        fill='none'
        aria-hidden='true'
    >
        <path
            d='M6 18.5V12.5M12 18.5V5.5M18 18.5V9.5'
            stroke='currentColor'
            strokeWidth='2.2'
            strokeLinecap='round'
        />
    </svg>
);

// Кнопка в панели форматирования: открывает то же окно, что и команда /poll.
// Никакой серверной логики здесь нет — команда исполняется штатным API,
// плагин на сервере сам открывает окно создания опроса.
const PollButton = (props) => {
    const [busy, setBusy] = useState(false);
    const channelId = currentChannelId();

    useEffect(injectStyles, []);

    const openDialog = useCallback(async () => {
        if (busy || !channelId) {
            return;
        }
        setBusy(true);

        const triggerId = randomTriggerId();
        try {
            const response = await fetch('/api/v4/commands/execute', {
                method: 'POST',
                credentials: 'same-origin',
                headers: {
                    'Content-Type': 'application/json',
                    'X-Requested-With': 'XMLHttpRequest',
                },
                body: JSON.stringify({
                    command: '/poll',
                    channel_id: channelId,
                    team_id: currentTeamId(),
                    trigger_id: triggerId,
                }),
            });
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }
            const data = await response.json();
            pluginStore.dispatch({
                type: RECEIVED_DIALOG_TRIGGER_ID,
                data: data.trigger_id || triggerId,
            });
        } catch (error) {
            // Окно не открылось — оставляем пользователю обычный путь: команда
            // в поле ввода, оттуда её отправит он сам.
            try {
                const message = (props && props.draft && props.draft.message) || '';
                if (props && typeof props.updateText === 'function') {
                    props.updateText(message ? `${message} /poll` : '/poll');
                }
            } catch (ignored) {
                // ничего не остаётся, кроме как молча закончить
            }
        } finally {
            setBusy(false);
        }
    }, [busy, channelId, props]);

    const title = channelId ? 'Создать опрос' : 'Опросы создаются в канале';

    return (
        <button
            type='button'
            className='polls-toolbar-button'
            aria-label={title}
            title={title}
            disabled={!channelId || busy}
            onClick={openDialog}
            data-testid='polls-toolbar-button'
        >
            <PollIcon/>
        </button>
    );
};

export default PollButton;

export const setStore = (store) => {
    pluginStore = store;
};
