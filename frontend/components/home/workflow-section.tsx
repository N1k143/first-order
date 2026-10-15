export function WorkflowSection() {
  return (
    <section className="workflow section" id="how">
      <div className="wrap workflow-grid">
        <div className="workflow-intro">
          <p className="eyebrow">От знакомства к проекту</p>
          <h2>
            Понятный порядок
            <br />
            работы.
          </h2>
          <p>
            First Order помогает найти повод для разговора. Условия проекта вы
            обсуждаете с компанией самостоятельно.
          </p>
          <a className="secondary-link" href="#catalog">
            Перейти к компаниям ↗
          </a>
        </div>
        <ol className="workflow-steps">
          <li>
            <span>01</span>
            <div>
              <h3>Изучите организацию</h3>
              <p>
                Посмотрите, какие услуги она предлагает и как сейчас общается с
                клиентами.
              </p>
            </div>
          </li>
          <li>
            <span>02</span>
            <div>
              <h3>Подготовьте предложение</h3>
              <p>
                Опишите конкретную пользу: показать работы, разместить меню или
                принимать заявки.
              </p>
            </div>
          </li>
          <li>
            <span>03</span>
            <div>
              <h3>Обсудите сотрудничество</h3>
              <p>
                Согласуйте объём работы, сроки и стоимость. Зафиксируйте
                договорённости перед началом проекта.
              </p>
            </div>
          </li>
        </ol>
      </div>
    </section>
  );
}
