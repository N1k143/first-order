import Link from 'next/link';
export default function NotFound() { return <section className="container page stack-lg"><h1>Страница не найдена</h1><p className="muted">Проверьте адрес или вернитесь на главную.</p><div><Link className="button" href="/">На главную</Link></div></section>; }
