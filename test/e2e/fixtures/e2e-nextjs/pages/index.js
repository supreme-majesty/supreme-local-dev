// Proves Next.js detection (`next dev -H 0.0.0.0 -p 3000`), npm install and routing.
export async function getServerSideProps() {
  return { props: { uid: process.getuid() } };
}

export default function Home({ uid }) {
  return <p>{`E2E-NEXTJS-OK uid=${uid}`}</p>;
}
