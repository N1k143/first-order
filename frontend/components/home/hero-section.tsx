export function HeroSection() {
  return (
    <section className="hero wrap">
      <div className="hero-copy">
        <p className="eyebrow">Для начинающих веб-разработчиков</p>
        <h1>
          Ваш первый заказ
          <br className="desktop-break" /> начинается
          <br className="desktop-break" /> с поиска компании.
        </h1>
        <p className="hero-description">
          Находите организации без сайта, изучайте их бизнес и предлагайте свои
          услуги напрямую.
        </p>
        <div className="hero-actions">
          <a className="button" href="#catalog">
            Найти компанию <span aria-hidden="true">↗</span>
          </a>
          <a className="secondary-link" href="#how">
            Как начать <span aria-hidden="true">↓</span>
          </a>
        </div>
      </div>
      <img
        className="hero-image"
        src="/first-order-hero-v2.png"
        width="1438"
        height="1094"
        alt="Обновлённый каталог First Order с поиском и выбором города на экране телефона"
        fetchPriority="high"
      />
    </section>
  );
}
