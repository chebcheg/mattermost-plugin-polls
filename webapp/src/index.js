import PollButton, {setStore} from './pollButton';
import {startPostStyles} from './postStyles';
import PollCard from './pollCard';

const pluginId = 'com.chebcheg.plugin-polls';

class Plugin {
    initialize(registry, store) {
        setStore(store);
        startPostStyles(store);

        // registerPostEditorActionComponent — кнопка в панели форматирования
        // (рядом с приоритетом и «сгорающим» сообщением). На серверах, где
        // метода ещё нет, плагин продолжает работать через команду /poll.
        if (typeof registry.registerPostEditorActionComponent === 'function') {
            registry.registerPostEditorActionComponent(PollButton);
        }

        // Карточка опроса: браузер и десктоп рисуют её вместо текста сообщения.
        // Мобильный клиент компонент не знает и показывает текст с кнопками.
        if (typeof registry.registerPostTypeComponent === 'function') {
            registry.registerPostTypeComponent('custom_polls', PollCard);
        }
    }

    uninitialize() {}
}

window.registerPlugin(pluginId, new Plugin());
