// Компактное оформление кнопок голосования.
//
// Mattermost рисует кнопки вложений своими стилями (высота 36 px, отступы
// 8/16 px), и опрос с длинными вариантами занимает пол-экрана. Правила здесь
// привязаны к конкретным сообщениям опросов по id поста, поэтому чужие кнопки
// в канале не затрагиваются. Если Mattermost переименует классы, правила
// просто перестанут применяться — вёрстка вернётся к штатной, ничего не сломается.

const STYLE_ID = 'polls-post-styles';
const PROP = 'polls_poll_id';

const rulesFor = (postId) => `
#post_${postId} .mm-blocks .btn {
    height: 28px !important;
    min-height: 28px !important;
    padding: 0 12px !important;
    font-size: 13px !important;
    line-height: 28px !important;
    border-radius: 4px !important;
}
#post_${postId} .mm-blocks-container { gap: 6px !important; }
#post_${postId} .mm-blocks-container--border { padding: 6px 8px !important; }
`;

const collectPostIds = (state) => {
    try {
        const posts = (state.entities && state.entities.posts && state.entities.posts.posts) || {};
        const ids = [];
        Object.keys(posts).forEach((key) => {
            const post = posts[key];
            if (post && post.id && post.props && post.props[PROP]) {
                ids.push(post.id);
            }
        });
        return ids;
    } catch (error) {
        return [];
    }
};

export const startPostStyles = (store) => {
    let timer = null;

    const apply = () => {
        timer = null;
        const css = collectPostIds(store.getState()).map(rulesFor).join('');

        let style = document.getElementById(STYLE_ID);
        if (!style) {
            style = document.createElement('style');
            style.id = STYLE_ID;
            document.head.appendChild(style);
        }
        const full = CARD_STYLES + css;
        if (style.textContent !== full) {
            style.textContent = full;
        }
    };

    const schedule = () => {
        if (timer !== null) {
            return;
        }
        timer = window.setTimeout(apply, 120);
    };

    apply();
    store.subscribe(schedule);

    // Лента подгружается асинхронно: ещё один прогон после старта, когда посты
    // уже в сторе.
    window.setTimeout(apply, 1500);
};

// Оформление карточки опроса (кастомный тип поста custom_polls).
export const CARD_STYLES = `
.polls-card {
    border: 1px solid rgba(var(--center-channel-color-rgb), 0.12);
    border-radius: 12px;
    padding: 12px 14px;
    max-width: 520px;
    background: rgba(var(--center-channel-color-rgb), 0.03);
    font-family: inherit;
}
.polls-card__question {
    font-size: 15px;
    font-weight: 600;
    line-height: 20px;
    color: var(--center-channel-color);
    overflow-wrap: anywhere;
}
.polls-card__kind {
    margin-top: 2px;
    font-size: 12px;
    color: rgba(var(--center-channel-color-rgb), 0.56);
}
.polls-card__rows { margin-top: 10px; display: flex; flex-direction: column; gap: 2px; }
.polls-card__row {
    display: flex;
    flex-direction: column;
    gap: 4px;
    width: 100%;
    padding: 6px 6px;
    border: none;
    border-radius: 6px;
    background: transparent;
    color: var(--center-channel-color);
    font-size: 14px;
    text-align: left;
    cursor: pointer;
}
.polls-card__row:not(.polls-card__row--result) { flex-direction: row; align-items: center; gap: 10px; }
.polls-card__row:hover:not(:disabled) { background: rgba(var(--center-channel-color-rgb), 0.06); }
.polls-card__row:disabled { cursor: default; }
.polls-card__result-top { display: flex; align-items: baseline; gap: 8px; }
.polls-card__percent { min-width: 42px; font-weight: 600; font-variant-numeric: tabular-nums; }
.polls-card__text { flex: 1; overflow-wrap: anywhere; }
.polls-card__count { font-size: 13px; color: rgba(var(--center-channel-color-rgb), 0.56); font-variant-numeric: tabular-nums; }
.polls-card__tick { color: var(--button-bg); font-weight: 700; }
.polls-card__box {
    width: 18px;
    height: 18px;
    flex: 0 0 18px;
    border: 2px solid rgba(var(--center-channel-color-rgb), 0.4);
    border-radius: 4px;
    position: relative;
}
.polls-card__box--radio { border-radius: 50%; }
.polls-card__box--on { border-color: var(--button-bg); background: var(--button-bg); }
.polls-card__box--on::after {
    content: '✓';
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 12px;
    line-height: 1;
    color: var(--button-color, #fff);
}
.polls-card__bar {
    height: 4px;
    border-radius: 2px;
    background: rgba(var(--center-channel-color-rgb), 0.12);
    overflow: hidden;
}
.polls-card__bar > span {
    display: block;
    height: 100%;
    border-radius: 2px;
    background: var(--button-bg);
}
.polls-card__submit-row { margin-top: 10px; display: flex; justify-content: flex-end; }
.polls-card__submit {
    height: 32px;
    padding: 0 16px;
    border: none;
    border-radius: 4px;
    background: var(--button-bg);
    color: var(--button-color, #fff);
    font-size: 14px;
    font-weight: 600;
    cursor: pointer;
}
.polls-card__submit:disabled { opacity: 0.6; cursor: default; }
.polls-card__footer {
    margin-top: 10px;
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    align-items: baseline;
    justify-content: space-between;
    font-size: 12px;
    color: rgba(var(--center-channel-color-rgb), 0.56);
}
.polls-card__links { display: flex; gap: 12px; }
.polls-card__link {
    border: none;
    background: transparent;
    padding: 0;
    font-size: 12px;
    color: var(--link-color, var(--button-bg));
    cursor: pointer;
}
.polls-card__link:hover { text-decoration: underline; }
.polls-card__error { margin-top: 8px; font-size: 12px; color: #d24b4e; }
.polls-card__voters {
    margin-top: 10px;
    padding-top: 8px;
    border-top: 1px solid rgba(var(--center-channel-color-rgb), 0.12);
    font-size: 12px;
    color: var(--center-channel-color);
}
.polls-card__voters-head { display: flex; justify-content: space-between; font-weight: 600; margin-bottom: 4px; }
.polls-card__voters-row { margin-top: 6px; }
.polls-card__voters-option { font-weight: 600; color: var(--center-channel-color); overflow-wrap: anywhere; }
.polls-card__voters-names {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
    padding-left: 12px;
    margin-top: 2px;
}
.polls-card__voter {
    padding: 1px 8px;
    border-radius: 10px;
    background: rgba(var(--center-channel-color-rgb), 0.08);
    color: rgba(var(--center-channel-color-rgb), 0.8);
    font-size: 12px;
    line-height: 18px;
    max-width: 100%;
    overflow-wrap: anywhere;
}
.polls-card__voter--empty { background: transparent; color: rgba(var(--center-channel-color-rgb), 0.56); padding-left: 0; }
/* На десктопе карточка заменяет текстовый фолбэк с кнопками. */
.polls-card-post .mm-blocks { display: none !important; }
`;
