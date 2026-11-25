import React, {useCallback, useEffect, useRef, useState} from 'react';

const PLUGIN_ID = 'com.chebcheg.plugin-polls';

const request = async (path, body) => {
    const options = {
        credentials: 'same-origin',
        headers: {'X-Requested-With': 'XMLHttpRequest'},
    };
    if (body) {
        options.method = 'POST';
        options.headers['Content-Type'] = 'application/json';
        options.body = JSON.stringify(body);
    }
    const response = await fetch(`/plugins/${PLUGIN_ID}/api/v1/card${path}`, options);
    const data = await response.json().catch(() => ({}));
    if (!response.ok && !data.error) {
        throw new Error(`HTTP ${response.status}`);
    }
    return data;
};

const plural = (n, one, few, many) => {
    const mod100 = n % 100;
    if (mod100 >= 11 && mod100 <= 14) {
        return `${n} ${many}`;
    }
    switch (n % 10) {
    case 1:
        return `${n} ${one}`;
    case 2:
    case 3:
    case 4:
        return `${n} ${few}`;
    default:
        return `${n} ${many}`;
    }
};

const PollCard = ({post}) => {
    const postId = post && post.id;
    const [state, setState] = useState(null);
    const [voters, setVoters] = useState(null);
    const [busy, setBusy] = useState(false);
    const [pending, setPending] = useState([]);
    const rootRef = useRef(null);

    // Скрываем текстовый фолбэк с кнопками: на десктопе его заменяет карточка.
    // Если компонент не смонтировался, класс не появится и фолбэк останется.
    useEffect(() => {
        const node = rootRef.current;
        const postElement = node && node.closest('.post');
        if (postElement) {
            postElement.classList.add('polls-card-post');
        }
        return () => {
            if (postElement) {
                postElement.classList.remove('polls-card-post');
            }
        };
    }, []);

    const load = useCallback(async () => {
        try {
            setState(await request(`?post_id=${encodeURIComponent(postId)}`));
        } catch (error) {
            setState({error: 'Не удалось загрузить опрос'});
        }
    }, [postId]);

    useEffect(() => {
        load();
    }, [load, post && post.update_at]);

    const act = useCallback(async (path, body) => {
        if (busy) {
            return;
        }
        setBusy(true);
        try {
            const data = await request(path, {post_id: postId, ...body});
            setState(data);
            setPending([]);
            setVoters(null);
        } catch (error) {
            setState((prev) => ({...(prev || {}), error: 'Не удалось сохранить голос'}));
        } finally {
            setBusy(false);
        }
    }, [busy, postId]);

    const voteFor = useCallback((state_, optionId) => {
        if (!state_ || state_.closed) {
            return;
        }
        if (state_.multiple && !state_.voted) {
            setPending((prev) => (prev.includes(optionId) ? prev.filter((id) => id !== optionId) : [...prev, optionId]));
            return;
        }
        act('/vote', {option_id: optionId});
    }, [act]);

    const submitPending = useCallback(() => {
        // Все отмеченные варианты уходят одним запросом.
        act('/vote', {option_ids: pending});
    }, [pending, act]);

    const loadVoters = useCallback(async () => {
        try {
            setVoters(await request(`/voters?post_id=${encodeURIComponent(postId)}`));
        } catch (error) {
            setVoters({error: 'Список недоступен'});
        }
    }, [postId]);

    if (!state) {
        return (
            <div className='polls-card is-loading' ref={rootRef}>
                <div className='polls-card__question'>Опрос</div>
                <div className='polls-card__kind'>загрузка…</div>
            </div>
        );
    }

    // Карточка ведёт себя как опрос в Телеграме: пока человек не проголосовал,
    // показываем варианты с флажками, а проценты — только после голоса (или
    // после завершения опроса).
    const showResults = Boolean(state.voted || state.closed);

    // Подпись собирается частями и склеивается через «·»: раньше две соседние
    // ветки давали «Опрос завершёнголосов пока нет».
    const metaParts = [];
    if (state.closed) {
        metaParts.push('Опрос завершён');
    } else if (!showResults) {
        metaParts.push('проголосуйте, чтобы увидеть результаты');
    }
    if (showResults) {
        metaParts.push(state.total_votes === 0 ? 'голосов пока нет' : `${plural(state.total_votes, 'голос', 'голоса', 'голосов')} · ${plural(state.voters_count, 'человек', 'человека', 'человек')}`);
    }
    const metaText = metaParts.join(' · ');
    const flags = ['Опрос'];
    if (state.multiple) {
        flags.push('несколько ответов');
    }
    if (state.anonymous) {
        flags.push('анонимное');
    }

    return (
        <div className='polls-card' ref={rootRef}>
            <div className='polls-card__question'>{state.question}</div>
            <div className='polls-card__kind'>{flags.join(' · ')}</div>

            <div className='polls-card__rows'>
                {state.options.map((option) => {
                    const chosen = pending.includes(option.id);
                    const mine = option.mine || chosen;

                    if (!showResults) {
                        return (
                            <button
                                key={option.id}
                                type='button'
                                className='polls-card__row'
                                disabled={!state.can_vote || busy}
                                onClick={() => voteFor(state, option.id)}
                            >
                                <span className={`polls-card__box ${state.multiple ? '' : 'polls-card__box--radio'} ${mine ? 'polls-card__box--on' : ''}`}/>
                                <span className='polls-card__text'>{option.text}</span>
                            </button>
                        );
                    }

                    return (
                        <button
                            key={option.id}
                            type='button'
                            className='polls-card__row polls-card__row--result'
                            disabled={!state.can_vote || busy}
                            onClick={() => voteFor(state, option.id)}
                            title={state.can_vote ? 'Изменить голос' : undefined}
                        >
                            <div className='polls-card__result-top'>
                                <span className='polls-card__percent'>{option.percent}%</span>
                                <span className='polls-card__text'>
                                    {option.text}
                                    {mine ? <span className='polls-card__tick'> ✓</span> : null}
                                </span>
                                <span className='polls-card__count'>{option.count}</span>
                            </div>
                            <div className='polls-card__bar'>
                                <span style={{width: `${option.percent}%`}}/>
                            </div>
                        </button>
                    );
                })}
            </div>

            {state.multiple && !state.voted && pending.length > 0 && (
                <div className='polls-card__submit-row'>
                    <button
                        type='button'
                        className='polls-card__submit'
                        disabled={busy}
                        onClick={submitPending}
                    >
                        Голосовать
                    </button>
                </div>
            )}

            <div className='polls-card__footer'>
                <span className='polls-card__meta'>{metaText}</span>
                <span className='polls-card__links'>
                    {!state.anonymous && state.total_votes > 0 && (state.voted || state.closed) && (
                        <button type='button' className='polls-card__link' onClick={loadVoters}>
                            Кто голосовал
                        </button>
                    )}
                    {state.voted && !state.closed && (
                        <button type='button' className='polls-card__link' onClick={() => act('/cancel', {})}>
                            Отменить голос
                        </button>
                    )}
                    {state.can_end && (
                        <button type='button' className='polls-card__link' onClick={() => act('/end', {})}>
                            Завершить
                        </button>
                    )}
                </span>
            </div>

            {state.error && <div className='polls-card__error'>{state.error}</div>}

            {voters && (
                <div className='polls-card__voters'>
                    {voters.error ? <div className='polls-card__error'>{voters.error}</div> : (
                        <>
                            <div className='polls-card__voters-head'>
                                Кто голосовал
                                <button type='button' className='polls-card__link' onClick={() => setVoters(null)}>закрыть</button>
                            </div>
                            {voters.options.map((option) => (
                                <div key={option.text} className='polls-card__voters-row'>
                                    <div className='polls-card__voters-option'>{option.text}</div>
                                    <div className='polls-card__voters-names'>
                                        {option.names.length ? option.names.map((name) => (
                                            <span key={name} className='polls-card__voter'>{name}</span>
                                        )) : <span className='polls-card__voter polls-card__voter--empty'>никто</span>}
                                        {option.more > 0 && (
                                            <span className='polls-card__voter polls-card__voter--empty'>и ещё {option.more}</span>
                                        )}
                                    </div>
                                </div>
                            ))}
                        </>
                    )}
                </div>
            )}
        </div>
    );
};

export default PollCard;
